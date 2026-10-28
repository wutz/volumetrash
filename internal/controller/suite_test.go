/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	projectv1 "github.com/wutz/project/api/v1alpha1"

	"github.com/wutz/volumetrash/internal/config"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

// projectModule is the module of the project operator. It owns the Project CRD,
// which the operator reads for each project's trash retention. Its reconcilers
// are not run here.
const projectModule = "github.com/wutz/project"

// platformNamespace holds the platform ConfigMap that forgeai renders.
const platformNamespace = "forgeai-system"

// testPlatformConfig is the forgeai platform config; the operator reads only its
// trashRetention, and the other keys are present to show they are ignored.
const testPlatformConfig = `
namespacePrefix: forge-
storageTypes:
  shared: {storageClass: shared}
defaultStorageType: shared
trashRetention: 24h
`

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(projectv1.AddToScheme(scheme.Scheme)).To(Succeed())

	crdDir, err := projectCRDDir()
	Expect(err).NotTo(HaveOccurred())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{crdDir},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// The platform ConfigMap and two project namespaces: one of a project that
	// sets a retention, and one with no project at all.
	Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: platformNamespace}})).To(Succeed())
	Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: config.DefaultConfigMapName, Namespace: platformNamespace},
		Data:       map[string]string{"config.yaml": testPlatformConfig},
	})).To(Succeed())
	Expect(k8sClient.Create(ctx, &projectv1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "demo"},
		Spec:       projectv1.ProjectSpec{TrashRetention: &metav1.Duration{Duration: time.Hour}},
	})).To(Succeed())
	Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "forge-demo", Labels: map[string]string{"project.wutz.dev/project": "demo"},
	}})).To(Succeed())
	Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "forge-plain"}})).To(Succeed())
	// Pods need the default ServiceAccount, which no controller creates in envtest.
	Expect(k8sClient.Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "forge-plain"}})).To(Succeed())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme.Scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect((&PersistentVolumeClaimReconciler{
		Client: mgr.GetClient(),
		Config: &config.Loader{Reader: mgr.GetAPIReader(), Namespace: platformNamespace, Name: config.DefaultConfigMapName},
	}).SetupWithManager(mgr)).To(Succeed())

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).To(Succeed())
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// projectCRDDir returns the CRD directory of the project module this build depends on.
func projectCRDDir() (string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", projectModule).Output()
	if err != nil {
		return "", fmt.Errorf("locate %s: %w", projectModule, err)
	}
	return filepath.Join(strings.TrimSpace(string(out)), "config", "crd", "bases"), nil
}

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
