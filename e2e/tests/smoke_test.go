package tests

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

const (
	Timeout  = time.Second * 600
	Interval = time.Second * 5

	ydbImage      = "cr.yandex/crptqonuodf51kdj7a7d/ydb:22.4.44"
	ydbNamespace  = "ydb"
	ydbHome       = "/home/ydb"
	storageName   = "storage"
	databaseName  = "database"
	defaultDomain = "Root"

	ReadyStatus = "Ready"
	// This is the directory that is used to store temporary files to be propagated into pods.
	// For example, in `hostPath` volume testing.
	// It works like this: for 8 kind workers, we create 8 folders: /tmp/worker-1/volume,
	// and each of them is propagated to /home/tmp/volume of each of the workers.
	// We have to make 8 copies on host kind runner, because duplicate mounts in docker
	// are not allowed.
	TmpFilesDir = "/home/tmp"
)

var HostPathDirectoryType corev1.HostPathType = "Directory"

func podIsReady(conditions []corev1.PodCondition) bool {
	for _, condition := range conditions {
		if condition.Type == ReadyStatus && condition.Status == "True" {
			return true
		}
	}
	return false
}

func constructAntiAffinityFor(key, value string) *corev1.Affinity {
	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      key,
								Operator: "In",
								Values:   []string{value},
							},
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}
}

func execInPod(namespace string, name string, cmd []string) (string, error) {
	args := []string{
		"-n",
		namespace,
		"exec",
		name,
		"--",
	}
	args = append(args, cmd...)
	result := exec.Command("kubectl", args...)
	stdout, err := result.Output()
	return string(stdout), err
}

func bringYdbCliToPod(namespace string, name string, ydbHome string) error {
	args := []string{
		"-n",
		namespace,
		"cp",
		fmt.Sprintf("%v/ydb/bin/ydb", os.ExpandEnv("$HOME")),
		fmt.Sprintf("%v:%v/ydb", name, ydbHome),
	}
	result := exec.Command("kubectl", args...)
	_, err := result.Output()
	return err
}

func installOperatorWithHelm(namespace string) bool {
	args := []string{
		"-n",
		namespace,
		"install",
		"ydb-operator",
		filepath.Join("..", "..", "deploy", "ydb-operator"),
		"-f",
		filepath.Join("..", "operator-values.yaml"),
	}
	result := exec.Command("helm", args...)
	stdout, err := result.Output()
	if err != nil {
		return false
	}

	return strings.Contains(string(stdout), "deployed")
}

func uninstallOperatorWithHelm(namespace string) bool {
	args := []string{
		"-n",
		namespace,
		"uninstall",
		"ydb-operator",
	}
	result := exec.Command("helm", args...)
	stdout, err := result.Output()
	if err != nil {
		return false
	}

	return strings.Contains(string(stdout), "uninstalled")
}

func makeVolumeForKindWorkers(nWorkers int, name string, content string) {
	for i := 1; i <= nWorkers; i++ {
		tmpVolumeDir := fmt.Sprintf("/tmp/worker-%v/volume", strconv.FormatInt(int64(i), 10))
		Expect(os.Mkdir(tmpVolumeDir, 0o777)).To(Succeed())

		sampleFile, err := os.Create(fmt.Sprintf("%v/%v", tmpVolumeDir, name))
		Expect(err).ToNot(HaveOccurred())

		bytesWritten, err := sampleFile.Write([]byte(content))
		Expect(bytesWritten).To(Equal(len(content)))
		Expect(err).ToNot(HaveOccurred())
	}
}

func cleanupFilesForKindWorkers(nWorkers int) {
	for i := 1; i <= nWorkers; i++ {
		tmpHostDir := fmt.Sprintf("/tmp/worker-%v/volume", strconv.FormatInt(int64(i), 10))
		Expect(os.RemoveAll(tmpHostDir)).Should(Succeed())
	}
}

var _ = Describe("Operator smoke test", func() {
	var ctx context.Context
	var namespace corev1.Namespace

	storageConfig, err := ioutil.ReadFile(filepath.Join(".", "data", "storage-block-4-2-config.yaml"))
	Expect(err).To(BeNil())

	storageAntiAffinity := constructAntiAffinityFor("ydb-cluster", "kind-storage")
	databaseAntiAffinity := constructAntiAffinityFor("ydb-cluster", "kind-database")

	defaultPolicy := corev1.PullIfNotPresent

	var storageSample *v1alpha1.Storage
	var databaseSample *v1alpha1.Database

	BeforeEach(func() {
		storageSample = &v1alpha1.Storage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      storageName,
				Namespace: ydbNamespace,
			},
			Spec: v1alpha1.StorageSpec{
				Nodes:         8,
				Configuration: string(storageConfig),
				Erasure:       "block-4-2",
				DataStore:     []corev1.PersistentVolumeClaimSpec{},
				Service: v1alpha1.StorageServices{
					GRPC: v1alpha1.GRPCService{
						TLSConfiguration: &v1alpha1.TLSConfiguration{
							Enabled: false,
						},
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
					},
					Interconnect: v1alpha1.InterconnectService{
						TLSConfiguration: &v1alpha1.TLSConfiguration{
							Enabled: false,
						},
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
					},
					Status: v1alpha1.StatusService{
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
					},
				},
				Domain:    defaultDomain,
				Resources: corev1.ResourceRequirements{},
				Image: v1alpha1.PodImage{
					Name:           ydbImage,
					PullPolicyName: &defaultPolicy,
				},
				AdditionalLabels: map[string]string{"ydb-cluster": "kind-storage"},
				Affinity:         storageAntiAffinity,
				Monitoring: &v1alpha1.MonitoringOptions{
					Enabled: false,
				},
			},
		}

		databaseSample = &v1alpha1.Database{
			ObjectMeta: metav1.ObjectMeta{
				Name:      databaseName,
				Namespace: ydbNamespace,
			},
			Spec: v1alpha1.DatabaseSpec{
				Nodes: 8,
				Resources: &v1alpha1.DatabaseResources{
					StorageUnits: []v1alpha1.StorageUnit{
						{
							UnitKind: "ssd",
							Count:    1,
						},
					},
				},
				Service: v1alpha1.DatabaseServices{
					GRPC: v1alpha1.GRPCService{
						TLSConfiguration: &v1alpha1.TLSConfiguration{
							Enabled: false,
						},
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
					},
					Interconnect: v1alpha1.InterconnectService{
						TLSConfiguration: &v1alpha1.TLSConfiguration{
							Enabled: false,
						},
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
					},
					Datastreams: v1alpha1.DatastreamsService{
						TLSConfiguration: &v1alpha1.TLSConfiguration{
							Enabled: false,
						},
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
					},
					Status: v1alpha1.StatusService{
						Service: v1alpha1.Service{IPFamilies: []corev1.IPFamily{"IPv4"}},
					},
				},
				Datastreams: &v1alpha1.DatastreamsConfig{
					Enabled: false,
				},
				Monitoring: &v1alpha1.MonitoringOptions{
					Enabled: false,
				},
				StorageClusterRef: v1alpha1.StorageRef{
					Name:      storageName,
					Namespace: ydbNamespace,
				},
				Domain: defaultDomain,
				Image: v1alpha1.PodImage{
					Name:           ydbImage,
					PullPolicyName: &defaultPolicy,
				},
				AdditionalLabels: map[string]string{"ydb-cluster": "kind-database"},
				Affinity:         databaseAntiAffinity,
			},
		}

		ctx = context.Background()
		namespace = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ydbNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, &namespace)).Should(Succeed())
		Expect(installOperatorWithHelm(ydbNamespace)).Should(BeTrue())
		time.Sleep(time.Second * 10)
	})

	It("general smoke pipeline, create storage + database", func() {
		By("issuing create commands...")
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()

		storage := v1alpha1.Storage{}
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: ydbNamespace,
			}, &storage)).Should(Succeed())

			return meta.IsStatusConditionPresentAndEqual(
				storage.Status.Conditions,
				"StorageReady",
				metav1.ConditionTrue,
			) && storage.Status.State == ReadyStatus
		}, Timeout, Interval).Should(BeTrue())

		By("checking that all the storage pods are running and ready...")
		storagePods := corev1.PodList{}
		Expect(k8sClient.List(ctx, &storagePods, client.InNamespace(ydbNamespace), client.MatchingLabels{
			"ydb-cluster": "kind-storage",
		})).Should(Succeed())
		Expect(len(storagePods.Items)).Should(BeEquivalentTo(storageSample.Spec.Nodes))
		for _, pod := range storagePods.Items {
			Expect(pod.Status.Phase).To(BeEquivalentTo("Running"))
			Expect(podIsReady(pod.Status.Conditions)).To(BeTrue())
		}

		By("waiting until database is ready...")
		database := v1alpha1.Database{}
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: ydbNamespace,
			}, &database)).Should(Succeed())
			return meta.IsStatusConditionPresentAndEqual(
				database.Status.Conditions,
				"TenantInitialized",
				metav1.ConditionTrue,
			) && database.Status.State == ReadyStatus
		}, Timeout, Interval).Should(BeTrue())

		By("checking that all the database pods are running and ready...")
		databasePods := corev1.PodList{}
		Expect(k8sClient.List(ctx, &databasePods, client.InNamespace(ydbNamespace), client.MatchingLabels{
			"ydb-cluster": "kind-database",
		})).Should(Succeed())
		Expect(len(databasePods.Items)).Should(BeEquivalentTo(databaseSample.Spec.Nodes))
		for _, pod := range databasePods.Items {
			Expect(pod.Status.Phase).To(BeEquivalentTo("Running"))
			Expect(podIsReady(pod.Status.Conditions)).To(BeTrue())
		}

		firstDBPod := databasePods.Items[0].Name

		Expect(bringYdbCliToPod(ydbNamespace, firstDBPod, ydbHome)).To(Succeed())

		out, err := execInPod(ydbNamespace, firstDBPod, []string{
			fmt.Sprintf("%v/ydb", ydbHome),
			"-d",
			"/" + defaultDomain,
			"-e",
			"grpc://localhost:2135",
			"yql",
			"-s",
			"select 1",
		})

		Expect(err).To(BeNil())

		// `yql` gives output in the following format:
		// ┌─────────┐
		// | column0 |
		// ├─────────┤
		// | 1       |
		// └─────────┘
		Expect(strings.ReplaceAll(out, "\n", "")).
			To(MatchRegexp(".*column0.*1.*"))
	})

	It("storage.State goes Pending -> Preparing -> Provisioning -> Initializing -> Ready", func() {
		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()

		By("waiting until storage is ready...")
		storage := v1alpha1.Storage{}
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: ydbNamespace,
			}, &storage)).Should(Succeed())

			return meta.IsStatusConditionPresentAndEqual(
				storage.Status.Conditions,
				"StorageReady",
				metav1.ConditionTrue,
			) && storage.Status.State == ReadyStatus
		}, Timeout, Interval).Should(BeTrue())

		By("tracking storage state changes...")
		events, err := clientset.CoreV1().Events(ydbNamespace).List(context.Background(),
			metav1.ListOptions{TypeMeta: metav1.TypeMeta{Kind: "Storage"}})
		Expect(err).ToNot(HaveOccurred())

		allowedChanges := map[string]string{
			"Pending":      "Preparing",
			"Preparing":    "Provisioning",
			"Provisioning": "Initializing",
			"Initializing": ReadyStatus,
		}
		re := regexp.MustCompile(`Storage moved from ([a-zA-Z]+) to ([a-zA-Z]+)`)
		for _, event := range events.Items {
			if event.Reason == "StatusChanged" {
				match := re.FindStringSubmatch(event.Message)
				Expect(allowedChanges[match[1]]).To(BeEquivalentTo(match[2]))
			}
		}
	})

	It("[storage|database].Spec.Volumes gets propagated into pods", func() {
		By("preparing test file...")
		tmpHostFilename := "sample-file"
		tmpFileContent := "abc"

		makeVolumeForKindWorkers(8, tmpHostFilename, tmpFileContent)
		defer func() {
			cleanupFilesForKindWorkers(8)
		}()

		By("issuing create commands...")

		storageSample.Spec.Volumes = append(storageSample.Spec.Volumes, &corev1.Volume{
			Name: "sample-volume",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("%v/volume", TmpFilesDir),
					Type: &HostPathDirectoryType,
				},
			},
		})

		databaseSample.Spec.Volumes = append(storageSample.Spec.Volumes, &corev1.Volume{
			Name: "sample-volume",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: fmt.Sprintf("%v/volume", TmpFilesDir),
					Type: &HostPathDirectoryType,
				},
			},
		})

		Expect(k8sClient.Create(ctx, storageSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, storageSample)).Should(Succeed())
		}()
		Expect(k8sClient.Create(ctx, databaseSample)).Should(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, databaseSample)).Should(Succeed())
		}()

		By("waiting until storage is ready...")
		storage := v1alpha1.Storage{}
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      storageSample.Name,
				Namespace: ydbNamespace,
			}, &storage)).Should(Succeed())
			return storage.Status.State == ReadyStatus
		}, Timeout, Interval).Should(BeTrue())

		storagePods := corev1.PodList{}
		Expect(k8sClient.List(ctx, &storagePods, client.InNamespace(ydbNamespace), client.MatchingLabels{
			"ydb-cluster": "kind-storage",
		})).Should(Succeed())

		By("checking the volume has propagated into a storage pod...")
		firstStoragePod := storagePods.Items[0]
		output, err := execInPod(storage.Namespace, firstStoragePod.Name, []string{
			"cat",
			fmt.Sprintf("%v/%v", "/opt/ydb/volumes/sample-volume", tmpHostFilename),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(output).To(Equal(tmpFileContent))

		By("waiting until database is ready...")
		database := v1alpha1.Database{}
		Eventually(func(g Gomega) bool {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      databaseSample.Name,
				Namespace: ydbNamespace,
			}, &database)).Should(Succeed())
			return database.Status.State == ReadyStatus
		}, Timeout, Interval).Should(BeTrue())

		databasePods := corev1.PodList{}
		Expect(k8sClient.List(ctx, &databasePods, client.InNamespace(ydbNamespace), client.MatchingLabels{
			"ydb-cluster": "kind-database",
		})).Should(Succeed())

		By("checking the volume has propagated into a database pod...")
		firstDatabasePod := databasePods.Items[0]
		output, err = execInPod(database.Namespace, firstDatabasePod.Name, []string{
			"cat",
			fmt.Sprintf("%v/%v", "/opt/ydb/volumes/sample-volume", tmpHostFilename),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(output).To(Equal(tmpFileContent))
	})

	AfterEach(func() {
		Expect(uninstallOperatorWithHelm(ydbNamespace)).Should(BeTrue())
		Expect(k8sClient.Delete(ctx, &namespace)).Should(Succeed())
		// TODO wait until namespace is deleted properly
		time.Sleep(40 * time.Second)
	})
})
