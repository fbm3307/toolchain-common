package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/codeready-toolchain/toolchain-common/pkg/template"
	. "github.com/codeready-toolchain/toolchain-common/pkg/test"
	templatev1 "github.com/openshift/api/template/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	authv1 "github.com/openshift/api/authorization/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestApplySingle(t *testing.T) {
	// given
	addToScheme(t)

	defaultService := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registration-service",
			Namespace: "toolchain-host-operator",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.2.3.4",
			Selector: map[string]string{
				"run": "registration-service",
			},
		},
	}

	modifiedService := defaultService.DeepCopyObject().(*corev1.Service)
	modifiedService.Spec.Selector["run"] = "all-services"

	defaultCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registration-service",
			Namespace: "toolchain-host-operator",
		},
		Data: map[string]string{
			"first-param": "first-value",
		},
	}

	modifiedCm := defaultCm.DeepCopyObject().(*corev1.ConfigMap)
	modifiedCm.Data["first-param"] = "second-value"

	t.Run("updates of Services", func(t *testing.T) {

		// given
		namespacedName := types.NamespacedName{Namespace: "toolchain-host-operator", Name: "registration-service"}

		t.Run("as corev1 objects", func(t *testing.T) {

			t.Run("when using forceUpdate=true", func(t *testing.T) {

				t.Run("it should not update when specs are same", func(t *testing.T) {
					// given
					cl, _ := newClient(t)
					obj := defaultService.DeepCopy()
					_, err := cl.ApplyObject(context.TODO(), obj, client.ForceUpdate(true))
					require.NoError(t, err)
					originalGeneration := obj.GetGeneration()

					// when updating with the same obj again
					createdOrChanged, err := cl.ApplyObject(context.TODO(), obj, client.ForceUpdate(true))

					// then
					require.NoError(t, err)
					assert.False(t, createdOrChanged) // resource was not updated on the server, so returned value is `false`
					updateGeneration := obj.GetGeneration()
					assert.Equal(t, originalGeneration, updateGeneration)
				})

				t.Run("it should not update when specs are same except ClusterIP", func(t *testing.T) {
					// given
					cl, _ := newClient(t)
					obj := defaultService.DeepCopy()
					_, err := cl.ApplyObject(context.TODO(), obj, client.ForceUpdate(true))
					require.NoError(t, err)
					originalGeneration := obj.GetGeneration()
					obj.Spec.ClusterIP = "" // modify for version to update
					// when updating with the same obj again
					createdOrChanged, err := cl.ApplyObject(context.TODO(), obj, client.ForceUpdate(true))

					// then
					require.NoError(t, err)
					assert.False(t, createdOrChanged) // resource was not updated on the server, so returned value is `false`
					updateGeneration := obj.GetGeneration()
					assert.Equal(t, originalGeneration, updateGeneration)
					assert.Equal(t, defaultService.Spec.ClusterIP, obj.Spec.ClusterIP)
				})

				t.Run("it should update when specs are different", func(t *testing.T) {
					// given
					cl, _ := newClient(t)
					obj := defaultService.DeepCopy()
					_, err := cl.ApplyObject(context.TODO(), obj, client.ForceUpdate(true))
					require.NoError(t, err)
					originalGeneration := obj.GetGeneration()

					// when updating with the modified obj
					modifiedObj := modifiedService.DeepCopy()
					modifiedObj.Spec.ClusterIP = ""
					createdOrChanged, err := cl.ApplyObject(context.TODO(), modifiedObj, client.ForceUpdate(true))

					// then
					require.NoError(t, err)
					assert.True(t, createdOrChanged) // resource was updated on the server, so returned value if `true`
					updateGeneration := modifiedObj.GetGeneration()
					assert.Equal(t, originalGeneration+1, updateGeneration)
					assert.NotEmpty(t, modifiedObj.Annotations[client.LastAppliedConfigurationAnnotationKey])
				})

				t.Run("it should update when specs are different including ClusterIP", func(t *testing.T) {
					// given
					cl, _ := newClient(t)
					obj := defaultService.DeepCopy()
					_, err := cl.ApplyObject(context.TODO(), obj, client.ForceUpdate(true))
					require.NoError(t, err)
					originalGeneration := obj.GetGeneration()

					// when updating with the modified obj
					modifiedObj := modifiedService.DeepCopy()
					modifiedObj.Spec.ClusterIP = ""
					createdOrChanged, err := cl.ApplyObject(context.TODO(), modifiedObj, client.ForceUpdate(true))

					// then
					require.NoError(t, err)
					assert.True(t, createdOrChanged) // resource was updated on the server, so returned value if `true`
					updateGeneration := modifiedObj.GetGeneration()
					assert.Equal(t, originalGeneration+1, updateGeneration)
					assert.Equal(t, defaultService.Spec.ClusterIP, obj.Spec.ClusterIP)
				})

				t.Run("when object is missing, it should create it", func(t *testing.T) {
					// given
					cl, cli := newClient(t)

					// when
					createdOrChanged, err := cl.ApplyRuntimeObject(context.TODO(), modifiedService.DeepCopyObject(), client.ForceUpdate(true), client.SetOwner(&appsv1.Deployment{}))

					// then
					require.NoError(t, err)
					assert.True(t, createdOrChanged)
					service := &corev1.Service{}
					err = cli.Get(context.TODO(), namespacedName, service)
					require.NoError(t, err)
					assert.Equal(t, "all-services", service.Spec.Selector["run"])
					assert.NotEmpty(t, service.OwnerReferences)
					assert.NotEmpty(t, service.Annotations[client.LastAppliedConfigurationAnnotationKey])
				})
			})

			t.Run("when using forceUpdate=false", func(t *testing.T) {

				t.Run("it should update when spec is different", func(t *testing.T) {
					// given
					cl, cli := newClient(t)
					_, err := cl.ApplyRuntimeObject(context.TODO(), defaultService.DeepCopyObject(), client.ForceUpdate(true))
					require.NoError(t, err)

					// when
					createdOrChanged, err := cl.ApplyRuntimeObject(context.TODO(), modifiedService.DeepCopyObject())

					// then
					require.NoError(t, err)
					assert.True(t, createdOrChanged)
					service := &corev1.Service{}
					err = cli.Get(context.TODO(), namespacedName, service)
					require.NoError(t, err)
					assert.Equal(t, "all-services", service.Spec.Selector["run"])
				})

				t.Run("it should not update when using same object", func(t *testing.T) {
					// given
					cl, _ := newClient(t)
					_, err := cl.ApplyRuntimeObject(context.TODO(), defaultService.DeepCopyObject(), client.ForceUpdate(true))
					require.NoError(t, err)

					// when
					createdOrChanged, err := cl.ApplyRuntimeObject(context.TODO(), defaultService.DeepCopyObject())

					// then
					require.NoError(t, err)
					assert.False(t, createdOrChanged)
				})

				t.Run("when object is missing, it should create it", func(t *testing.T) {
					// given
					cl, cli := newClient(t)

					// when
					createdOrChanged, err := cl.ApplyRuntimeObject(context.TODO(), modifiedService.DeepCopyObject(), client.SetOwner(&appsv1.Deployment{}))

					// then
					require.NoError(t, err)
					assert.True(t, createdOrChanged)
					service := &corev1.Service{}
					err = cli.Get(context.TODO(), namespacedName, service)
					require.NoError(t, err)
					assert.Equal(t, "all-services", service.Spec.Selector["run"])
					assert.NotEmpty(t, service.OwnerReferences)
				})
			})

			t.Run("when not saving the configuration", func(t *testing.T) {

				t.Run("when object is missing, it should create it", func(t *testing.T) {
					// given
					cl, cli := newClient(t)

					// when
					createdOrChanged, err := cl.ApplyRuntimeObject(context.TODO(), modifiedService.DeepCopyObject(), client.SaveConfiguration(false))

					// then
					require.NoError(t, err)
					assert.True(t, createdOrChanged)
					service := &corev1.Service{}
					err = cli.Get(context.TODO(), namespacedName, service)
					require.NoError(t, err)
					assert.Equal(t, "all-services", service.Spec.Selector["run"])
					assert.Empty(t, service.Annotations[client.LastAppliedConfigurationAnnotationKey])
				})

				t.Run("it should update when spec is different", func(t *testing.T) {
					// given
					cl, cli := newClient(t)
					_, err := cl.ApplyRuntimeObject(context.TODO(), defaultService.DeepCopyObject(), client.SaveConfiguration(false))
					require.NoError(t, err)

					// when
					createdOrChanged, err := cl.ApplyRuntimeObject(context.TODO(), modifiedService.DeepCopyObject(), client.SaveConfiguration(false))

					// then
					require.NoError(t, err)
					assert.True(t, createdOrChanged)
					service := &corev1.Service{}
					err = cli.Get(context.TODO(), namespacedName, service)
					require.NoError(t, err)
					assert.Equal(t, "all-services", service.Spec.Selector["run"])
					assert.Empty(t, service.Annotations[client.LastAppliedConfigurationAnnotationKey])
				})
			})

			t.Run("when object cannot be retrieved because of any error, then it should fail", func(t *testing.T) {
				// given
				cl, cli := newClient(t)
				cli.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
					return fmt.Errorf("unable to get")
				}

				// when
				createdOrChanged, err := cl.ApplyRuntimeObject(context.TODO(), modifiedService.DeepCopyObject())

				// then
				require.Error(t, err)
				assert.False(t, createdOrChanged)
				assert.Contains(t, err.Error(), "unable to get the resource")
			})
		})

		t.Run("as unstructured objects", func(t *testing.T) {

			// only testing the specific cases of Services where an existing version exists, with a `spec.clusterIP` set
			// and the updated version has no value for this field

			t.Run("when using forceUpdate=true", func(t *testing.T) {

				t.Run("it should not update when specs are same except ClusterIP", func(t *testing.T) {
					// given
					cl, _ := newClient(t)
					// convert to unstructured
					obj, err := toUnstructured(defaultService.DeepCopy())

					require.NoError(t, err)
					_, err = cl.ApplyObject(context.TODO(), obj, client.ForceUpdate(true))
					require.NoError(t, err)
					modifiedObj := obj.DeepCopy()
					err = unstructured.SetNestedField(modifiedObj.Object, "", "spec", "clusterIP") // modify for version to update
					require.NoError(t, err)

					// when updating with the same obj again
					createdOrChanged, err := cl.ApplyObject(context.TODO(), modifiedObj, client.ForceUpdate(true))

					// then
					require.NoError(t, err)
					assert.False(t, createdOrChanged) // resource was not updated on the server, so returned value is `false`
					assert.Equal(t, obj.GetGeneration(), modifiedObj.GetGeneration())
					clusterIP, found, err := unstructured.NestedString(modifiedObj.Object, "spec", "clusterIP")
					require.NoError(t, err)
					require.True(t, found)
					assert.Equal(t, defaultService.Spec.ClusterIP, clusterIP)
				})
			})
		})
	})

	t.Run("updates of ConfigMaps", func(t *testing.T) {

		t.Run("it should update ConfigMap when data field is different and forceUpdate=false", func(t *testing.T) {
			// given
			cl, cli := newClient(t)
			_, err := cl.ApplyRuntimeObject(context.TODO(), defaultCm.DeepCopyObject(), client.ForceUpdate(true))
			require.NoError(t, err)

			// when
			createdOrChanged, err := cl.ApplyRuntimeObject(context.TODO(), modifiedCm.DeepCopyObject())

			// then
			require.NoError(t, err)
			assert.True(t, createdOrChanged)
			configMap := &corev1.ConfigMap{}
			namespacedName := types.NamespacedName{Namespace: "toolchain-host-operator", Name: "registration-service"}
			err = cli.Get(context.TODO(), namespacedName, configMap)
			require.NoError(t, err)
			assert.Equal(t, "second-value", configMap.Data["first-param"])
		})
	})

	t.Run("updates of ServiceAccount", func(t *testing.T) {

		t.Run("last-applied-configuration is used and SA secret ref is not updated", func(t *testing.T) {
			// given
			// there's an existing SA with secret refs
			existingSA := newSA()
			secretRefs := []corev1.ObjectReference{
				{
					Name:      "secret",
					Namespace: existingSA.Namespace,
				},
			}
			existingSA.Secrets = secretRefs
			existingLastAppliedAnnotation := map[string]string{
				client.LastAppliedConfigurationAnnotationKey: client.GetNewConfiguration(existingSA),
			}
			existingSA.SetAnnotations(existingLastAppliedAnnotation) // let's set the last applied annotation
			cl, cli := newClient(t)
			_, err := cl.ApplyRuntimeObject(context.TODO(), existingSA.DeepCopyObject())
			require.NoError(t, err)

			// when
			// we update with existing annotations
			newSA := existingSA.DeepCopy()
			newSA.SetAnnotations(existingLastAppliedAnnotation)   // let's set the last applied annotation
			_, err = cl.ApplyRuntimeObject(context.TODO(), newSA) // then

			// then
			require.NoError(t, err)
			var actualSa corev1.ServiceAccount
			err = cli.Get(context.TODO(), types.NamespacedName{Name: "appstudio-user-sa", Namespace: "john-dev"}, &actualSa) // assert sa was created
			require.NoError(t, err)
			assert.Equal(t, secretRefs, actualSa.Secrets)                                                                                                                    // secret refs are still there
			assert.Equal(t, existingLastAppliedAnnotation[client.LastAppliedConfigurationAnnotationKey], actualSa.Annotations[client.LastAppliedConfigurationAnnotationKey]) // the last apply configuration should match the previous object
		})
	})

	t.Run("should update object when save configuration is disabled and another annotation is present", func(t *testing.T) {
		// given
		cl, cli := newClient(t)
		existingRules := []rbac.PolicyRule{
			{
				APIGroups: []string{"toolchain.dev.openshift.com"},
				Resources: []string{"bannedusers", "masteruserrecords"},
				Verbs:     []string{"*"},
			},
		}
		existingRole := rbac.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "toolchaincluster-host",
				Namespace:   HostOperatorNs,
				Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"rbac.authorization.k8s.io/v1\",\"kind\":\"Role\",\"metadata\":{\"annotations\":{},\"name\":\"toolchaincluster-host\",\"namespace\":\"toolchain-host-operator\"},\"rules\":[{\"apiGroups\":[\"toolchain.dev.openshift.com\"],\"resources\":[\"bannedusers\",\"masteruserrecords\"],\"verbs\":[\"*\"]}]}"},
			},
			Rules: existingRules,
		}

		// when
		_, err := cl.ApplyRuntimeObject(context.TODO(), &existingRole, client.SaveConfiguration(false))
		require.NoError(t, err)

		// then
		// the object should match the existing role
		foundRole := &rbac.Role{}
		err = cli.Get(context.TODO(), types.NamespacedName{
			Name:      "toolchaincluster-host",
			Namespace: HostOperatorNs,
		}, foundRole)
		require.NoError(t, err)
		require.Equal(t, existingRules, foundRole.Rules)

		// when
		// we updated the role
		newRules := []rbac.PolicyRule{
			{
				APIGroups: []string{"toolchain.dev.openshift.com"},
				Resources: []string{"*"},
				Verbs:     []string{"*"},
			},
		}
		newRole := rbac.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "toolchaincluster-host",
				Namespace: HostOperatorNs,
			},
			Rules: newRules,
		}
		_, err = cl.ApplyRuntimeObject(context.TODO(), &newRole, client.SaveConfiguration(false))
		require.NoError(t, err)

		// then
		// the new rules should be there
		foundRole = &rbac.Role{}
		err = cl.Get(context.TODO(), types.NamespacedName{
			Name:      "toolchaincluster-host",
			Namespace: HostOperatorNs,
		}, foundRole)
		require.NoError(t, err)
		require.Equal(t, newRules, foundRole.Rules)
	})
}

func toUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	content, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	result := &unstructured.Unstructured{}
	_, _, err = unstructured.UnstructuredJSONScheme.Decode(content, nil, result)
	return result, err
}

func TestRetainClusterIP(t *testing.T) {

	t.Run("when new object is a service", func(t *testing.T) {

		t.Run("when existing object has a ClusterIP", func(t *testing.T) {
			// given
			newResource := &corev1.Service{
				Spec: corev1.ServiceSpec{},
			}
			existing := &corev1.Service{
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.2.3.4",
				},
			}

			// when
			err := client.RetainClusterIP(newResource, existing)

			// then
			require.NoError(t, err)
			assert.Equal(t, "10.2.3.4", newResource.Spec.ClusterIP)
		})

		t.Run("when existing object has no ClusterIP", func(t *testing.T) {
			// given
			newResource := &corev1.Service{
				Spec: corev1.ServiceSpec{},
			}
			existing := &corev1.Service{
				Spec: corev1.ServiceSpec{},
			}

			// when
			err := client.RetainClusterIP(newResource, existing)

			// then
			require.NoError(t, err)
			assert.Empty(t, newResource.Spec.ClusterIP)
		})

		t.Run("when existing object is not a service", func(t *testing.T) {
			// given
			newResource := &corev1.Service{
				Spec: corev1.ServiceSpec{},
			}
			existing := &corev1.ConfigMap{}

			// when
			err := client.RetainClusterIP(newResource, existing)

			// then
			require.NoError(t, err)
			assert.Empty(t, newResource.Spec.ClusterIP)
		})
	})

	t.Run("when new object is unstructured", func(t *testing.T) {

		t.Run("when existing object has a ClusterIP", func(t *testing.T) {
			// given
			newResource, err := toUnstructured(&corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind: "Service",
				},
				Spec: corev1.ServiceSpec{},
			})
			require.NoError(t, err)
			existing, err := toUnstructured(&corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind: "Service",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.2.3.4",
				},
			})
			require.NoError(t, err)

			// when
			err = client.RetainClusterIP(newResource, existing)

			// then
			require.NoError(t, err)
			clusterIP, found, err := unstructured.NestedString(newResource.Object, "spec", "clusterIP")
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, "10.2.3.4", clusterIP)
		})

		t.Run("when existing object has no ClusterIP", func(t *testing.T) {
			// given
			newResource, err := toUnstructured(&corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind: "Service",
				},
				Spec: corev1.ServiceSpec{},
			})
			require.NoError(t, err)
			existing, err := toUnstructured(&corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind: "Service",
				},
				Spec: corev1.ServiceSpec{},
			})
			require.NoError(t, err)

			// when
			err = client.RetainClusterIP(newResource, existing)

			// then
			require.NoError(t, err)
			_, found, err := unstructured.NestedString(newResource.Object, "spec", "clusterIP")
			require.NoError(t, err)
			assert.False(t, found)

		})

		t.Run("when existing object is not a service", func(t *testing.T) {
			// given
			newResource, err := toUnstructured(&corev1.Service{
				TypeMeta: metav1.TypeMeta{
					Kind: "Service",
				},
				Spec: corev1.ServiceSpec{},
			})
			require.NoError(t, err)
			existing, err := toUnstructured(&corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind: "ConfigMap",
				},
			})
			require.NoError(t, err)

			// when
			err = client.RetainClusterIP(newResource, existing)

			// then
			require.NoError(t, err)
			_, found, err := unstructured.NestedString(newResource.Object, "spec", "clusterIP")
			require.NoError(t, err)
			assert.False(t, found)
		})

	})

	t.Run("when new object is not a service nor unstructured", func(t *testing.T) {
		// given
		newResource := &corev1.ConfigMap{}
		existing := &corev1.ConfigMap{}

		// when
		err := client.RetainClusterIP(newResource, existing)

		// then
		require.NoError(t, err)
	})
}

func TestProcessAndApply(t *testing.T) {

	commit := getNameWithTimestamp("sha")
	user := getNameWithTimestamp("user")

	s := addToScheme(t)
	codecFactory := serializer.NewCodecFactory(s)
	decoder := codecFactory.UniversalDeserializer()

	values := map[string]string{
		"USERNAME": user,
		"COMMIT":   commit,
	}

	t.Run("should create namespace alone", func(t *testing.T) {
		// given
		cl := NewFakeClient(t)
		p := template.NewProcessor(s)
		tmpl, err := DecodeTemplate(decoder,
			CreateTemplate(WithObjects(Namespace), WithParams(UsernameParam, CommitParam)))
		require.NoError(t, err)
		objs, err := p.Process(tmpl, values)
		require.NoError(t, err)
		labels := newLabels("", "john", "")

		// when
		createdOrUpdated, err := client.NewApplyClient(cl).Apply(context.TODO(), objs, labels)

		// then
		require.NoError(t, err)
		assert.True(t, createdOrUpdated)
		assertNamespaceExists(t, cl, user, labels, commit)
	})

	t.Run("should create role binding alone", func(t *testing.T) {
		// given
		cl := NewFakeClient(t)
		p := template.NewProcessor(s)
		tmpl, err := DecodeTemplate(decoder,
			CreateTemplate(WithObjects(RoleBinding), WithParams(UsernameParam, CommitParam)))
		require.NoError(t, err)
		objs, err := p.Process(tmpl, values)
		require.NoError(t, err)
		labels := newLabels("base1ns", "john", "dev")

		// when
		createdOrUpdated, err := client.NewApplyClient(cl).Apply(context.TODO(), objs, labels)

		// then
		require.NoError(t, err)
		assert.True(t, createdOrUpdated)
		assertRoleBindingExists(t, cl, user, labels)
	})

	t.Run("should create namespace and role binding", func(t *testing.T) {
		// given
		cl := NewFakeClient(t)
		p := template.NewProcessor(s)
		tmpl, err := DecodeTemplate(decoder,
			CreateTemplate(WithObjects(Namespace, RoleBinding), WithParams(UsernameParam, CommitParam)))
		require.NoError(t, err)
		objs, err := p.Process(tmpl, values)
		require.NoError(t, err)
		labels := newLabels("", "john", "dev")

		// when
		createdOrUpdated, err := client.NewApplyClient(cl).Apply(context.TODO(), objs, labels)

		// then
		require.NoError(t, err)
		assert.True(t, createdOrUpdated)
		assertNamespaceExists(t, cl, user, labels, commit)
		assertRoleBindingExists(t, cl, user, labels)
	})

	t.Run("should update existing role binding", func(t *testing.T) {
		// given
		cl := NewFakeClient(t)
		cl.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
			t.Logf("updating resource of kind %s with version %s\n", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetResourceVersion())
			if obj.GetObjectKind().GroupVersionKind().Kind == "RoleBinding" && obj.GetResourceVersion() != "1" {
				return fmt.Errorf("invalid resource version: %q", obj.GetResourceVersion())
			}
			return cl.Client.Update(ctx, obj, opts...)
		}
		p := template.NewProcessor(s)
		tmpl, err := DecodeTemplate(decoder,
			CreateTemplate(WithObjects(RoleBinding), WithParams(UsernameParam, CommitParam)))
		require.NoError(t, err)
		objs, err := p.Process(tmpl, values)
		require.NoError(t, err)
		witoutType := newLabels("base1ns", "john", "")

		createdOrUpdated, err := client.NewApplyClient(cl).Apply(context.TODO(), objs, witoutType)
		require.NoError(t, err)
		assert.True(t, createdOrUpdated)
		assertRoleBindingExists(t, cl, user, witoutType)

		// when rolebinding changes
		tmpl, err = DecodeTemplate(decoder,
			CreateTemplate(WithObjects(Namespace, RoleBindingWithExtraUser), WithParams(UsernameParam, CommitParam)))
		require.NoError(t, err)
		objs, err = p.Process(tmpl, values)
		require.NoError(t, err)
		complete := newLabels("advanced", "john", "dev")
		createdOrUpdated, err = client.NewApplyClient(cl).Apply(context.TODO(), objs, complete)

		// then
		require.NoError(t, err)
		assert.True(t, createdOrUpdated)
		binding := assertRoleBindingExists(t, cl, user, complete)
		require.Len(t, binding.Subjects, 2)
		assert.Equal(t, "User", binding.Subjects[0].Kind)
		assert.Equal(t, user, binding.Subjects[0].Name)
		assert.Equal(t, "User", binding.Subjects[1].Kind)
		assert.Equal(t, "extraUser", binding.Subjects[1].Name)
	})

	t.Run("should not create or update existing namespace and role binding", func(t *testing.T) {
		// given
		cl := NewFakeClient(t)
		p := template.NewProcessor(s)
		tmpl, err := DecodeTemplate(decoder,
			CreateTemplate(WithObjects(Namespace, RoleBinding), WithParams(UsernameParam, CommitParam)))
		require.NoError(t, err)
		objs, err := p.Process(tmpl, values)
		require.NoError(t, err)
		labels := newLabels("base1ns", "john", "dev")
		created, err := client.NewApplyClient(cl).Apply(context.TODO(), objs, labels)
		require.NoError(t, err)
		assert.True(t, created)
		assertNamespaceExists(t, cl, user, labels, commit)
		assertRoleBindingExists(t, cl, user, labels)

		// when apply the same template again
		updated, err := client.NewApplyClient(cl).Apply(context.TODO(), objs, labels)

		// then
		require.NoError(t, err)
		assert.False(t, updated)
	})

	t.Run("failures", func(t *testing.T) {

		t.Run("should fail to create template object", func(t *testing.T) {
			// given
			cl := NewFakeClient(t)
			p := template.NewProcessor(s)
			cl.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
				return errors.New("failed to create resource")
			}
			tmpl, err := DecodeTemplate(decoder,
				CreateTemplate(WithObjects(RoleBinding), WithParams(UsernameParam, CommitParam)))
			require.NoError(t, err)

			// when
			objs, err := p.Process(tmpl, values)
			require.NoError(t, err)
			createdOrUpdated, err := client.NewApplyClient(cl).Apply(context.TODO(), objs, newLabels("", "", ""))

			// then
			require.Error(t, err)
			assert.False(t, createdOrUpdated)
		})

		t.Run("should fail to update template object", func(t *testing.T) {
			// given
			cl := NewFakeClient(t)
			p := template.NewProcessor(s)
			cl.MockUpdate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.UpdateOption) error {
				return errors.New("failed to update resource")
			}
			tmpl, err := DecodeTemplate(decoder,
				CreateTemplate(WithObjects(RoleBinding), WithParams(UsernameParam, CommitParam)))
			require.NoError(t, err)
			objs, err := p.Process(tmpl, values)
			require.NoError(t, err)
			labels := newLabels("", "", "")
			createdOrUpdated, err := client.NewApplyClient(cl).Apply(context.TODO(), objs, labels)
			require.NoError(t, err)
			assert.True(t, createdOrUpdated)

			// when
			tmpl, err = DecodeTemplate(decoder,
				CreateTemplate(WithObjects(RoleBindingWithExtraUser), WithParams(UsernameParam, CommitParam)))
			require.NoError(t, err)
			objs, err = p.Process(tmpl, values)
			require.NoError(t, err)
			createdOrUpdated, err = client.NewApplyClient(cl).Apply(context.TODO(), objs, newLabels("advanced", "john", "dev"))

			// then
			require.Error(t, err)
			assert.False(t, createdOrUpdated)
		})
	})

	t.Run("should create with extra newLabels and ownerref", func(t *testing.T) {

		// given
		values := map[string]string{
			"USERNAME": user,
			"COMMIT":   commit,
		}
		cl := NewFakeClient(t)
		p := template.NewProcessor(s)
		tmpl, err := DecodeTemplate(decoder,
			CreateTemplate(WithObjects(Namespace, RoleBinding), WithParams(UsernameParam, CommitParam)))
		require.NoError(t, err)
		objs, err := p.Process(tmpl, values)
		require.NoError(t, err)

		// when adding newLabels and an owner reference
		objs[0].SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: "crt/v1",
				Kind:       "NSTemplateSet",
				Name:       "foo",
			},
		})
		labels := newLabels("base1ns", "john", "dev")
		createdOrUpdated, err := client.NewApplyClient(cl).Apply(context.TODO(), objs, labels)

		// then
		require.NoError(t, err)
		assert.True(t, createdOrUpdated)
		ns := assertNamespaceExists(t, cl, user, labels, commit)
		// verify owner refs
		assert.Equal(t, []metav1.OwnerReference{
			{
				APIVersion: "crt/v1",
				Kind:       "NSTemplateSet",
				Name:       "foo",
			},
		}, ns.OwnerReferences)
	})
}

func TestApplyUnstructuredObjects(t *testing.T) {

	t.Run("special case - service account", func(t *testing.T) {
		t.Run("should create service account from unstructured object", func(t *testing.T) {
			// given
			sa := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "ServiceAccount",
					"metadata": map[string]interface{}{
						"name":      "toolchaincluster-member",
						"namespace": "toolchain-member-operator",
					},
					"apiVersion": "v1",
				},
			}
			cl := NewFakeClient(t)

			// when
			err := client.ApplyUnstructuredObjectsWithNewLabels(context.TODO(), cl, []*unstructured.Unstructured{sa}, map[string]string{toolchainv1alpha1.ProviderLabelKey: toolchainv1alpha1.ProviderLabelValue})
			require.NoError(t, err)

			// then
			var actualSa corev1.ServiceAccount
			err = cl.Get(context.TODO(), types.NamespacedName{Name: "toolchaincluster-member", Namespace: "toolchain-member-operator"}, &actualSa) // assert sa was created
			require.NoError(t, err)
			assert.Equal(t, toolchainv1alpha1.ProviderLabelValue, actualSa.Labels[toolchainv1alpha1.ProviderLabelKey])
		})

		t.Run("should update service account", func(t *testing.T) {
			// given
			// there's an existing SA with secret refs
			existingSA := newSA()
			secretRefs := []corev1.ObjectReference{
				{
					Name:      "secret",
					Namespace: existingSA.Namespace,
				},
			}
			existingSA.Secrets = secretRefs

			newSA := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind": "ServiceAccount",
					"metadata": map[string]interface{}{
						"name":      "appstudio-user-sa",
						"namespace": "john-dev",
					},
					"apiVersion": "v1",
				},
			}
			cl := NewFakeClient(t, existingSA)

			// when
			// we update the labels
			err := client.ApplyUnstructuredObjectsWithNewLabels(context.TODO(), cl, []*unstructured.Unstructured{newSA},
				map[string]string{toolchainv1alpha1.ProviderLabelKey: toolchainv1alpha1.ProviderLabelValue,
					toolchainv1alpha1.OwnerLabelKey: "someowner",
				},
			)
			require.NoError(t, err)

			// then
			var actualSa corev1.ServiceAccount
			err = cl.Get(context.TODO(), types.NamespacedName{Name: "appstudio-user-sa", Namespace: "john-dev"}, &actualSa) // assert sa was created
			require.NoError(t, err)
			assert.Equal(t, toolchainv1alpha1.ProviderLabelValue, actualSa.Labels[toolchainv1alpha1.ProviderLabelKey]) // existing label is still there
			assert.Equal(t, "someowner", actualSa.Labels[toolchainv1alpha1.OwnerLabelKey])                             // new label is here as well
			assert.Equal(t, secretRefs, actualSa.Secrets)                                                              // secret refs are still there
		})
	})

	t.Run("should apply other type of objects from unstructured object", func(t *testing.T) {
		// given
		role := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": "Role",
				"metadata": map[string]interface{}{
					"name":      "toolchaincluster-member",
					"namespace": "toolchain-member-operator",
				},
				"apiVersion": "rbac.authorization.k8s.io/v1",
			},
		}
		cl := NewFakeClient(t)

		// when
		err := client.ApplyUnstructuredObjectsWithNewLabels(context.TODO(), cl, []*unstructured.Unstructured{role}, map[string]string{toolchainv1alpha1.ProviderLabelKey: toolchainv1alpha1.ProviderLabelValue})

		// then
		require.NoError(t, err)
		var actualRole rbac.Role
		err = cl.Get(context.TODO(), types.NamespacedName{Name: "toolchaincluster-member", Namespace: "toolchain-member-operator"}, &actualRole) // assert role was created
		require.NoError(t, err)
		assert.Equal(t, toolchainv1alpha1.ProviderLabelValue, actualRole.Labels[toolchainv1alpha1.ProviderLabelKey])
	})
}

func TestMergeLabels(t *testing.T) {
	// given
	additionalLabel := map[string]string{
		"new": "label",
	}

	t.Run("original object has labels", func(t *testing.T) {
		// when
		sa := newSA()
		// SA has some initial labels
		sa.Labels = map[string]string{
			"foo": "bar",
		}
		client.MergeLabels(sa, additionalLabel)

		// then
		assert.True(t, reflect.DeepEqual(sa.Labels, map[string]string{"new": "label", "foo": "bar"}))
	})

	t.Run("original object has no labels", func(t *testing.T) {
		// when
		sa := newSA()
		client.MergeLabels(sa, additionalLabel)

		// then
		assert.True(t, reflect.DeepEqual(sa.Labels, additionalLabel))
	})
}

func TestMergeAnnotations(t *testing.T) {
	// given
	additionalAnnotation := map[string]string{
		"new": "annotation",
	}

	t.Run("original object has annotations", func(t *testing.T) {
		// when
		sa := newSA()
		// SA has some initial annotations
		sa.Annotations = map[string]string{
			"foo": "bar",
		}
		client.MergeAnnotations(sa, additionalAnnotation)

		// then
		assert.True(t, reflect.DeepEqual(sa.Annotations, map[string]string{"new": "annotation", "foo": "bar"}))
	})

	t.Run("original object has no annotations", func(t *testing.T) {
		// when
		sa := newSA()
		client.MergeAnnotations(sa, additionalAnnotation)

		// then
		assert.True(t, reflect.DeepEqual(sa.Annotations, additionalAnnotation))
	})
}

func newSA() *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "appstudio-user-sa",
			Namespace: "john-dev",
		},
	}
	return sa
}

func assertNamespaceExists(t *testing.T, c runtimeclient.Client, nsName string, labels map[string]string, version string) corev1.Namespace {
	// check that the namespace was created
	var ns corev1.Namespace
	err := c.Get(context.TODO(), types.NamespacedName{Name: nsName, Namespace: ""}, &ns) // assert namespace is cluster-scoped
	require.NoError(t, err)
	assert.Equal(t, expectedLabels(labels, version), ns.GetLabels())
	return ns
}

func expectedLabels(labels map[string]string, version string) map[string]string {
	expLabels := map[string]string{
		"extra": "something-extra",
	}
	if version != "" {
		expLabels["version"] = version
	}
	for k, v := range labels {
		expLabels[k] = v
	}
	return expLabels
}

func assertRoleBindingExists(t *testing.T, c runtimeclient.Client, ns string, labels map[string]string) authv1.RoleBinding {
	// check that the rolebinding is created in the namespace
	// (the fake client just records the request but does not perform any consistency check)
	var rb authv1.RoleBinding
	err := c.Get(context.TODO(), types.NamespacedName{
		Namespace: ns,
		Name:      fmt.Sprintf("%s-edit", ns),
	}, &rb)

	require.NoError(t, err)
	assert.Equal(t, expectedLabels(labels, ""), rb.GetLabels())
	return rb
}

func newLabels(tier, username, nsType string) map[string]string {
	labels := map[string]string{
		"toolchain.dev.openshift.com/provider": "codeready-toolchain",
	}
	if tier != "" {
		labels["toolchain.dev.openshift.com/tier"] = tier
	}
	if username != "" {
		labels["toolchain.dev.openshift.com/owner"] = username
	}
	if nsType != "" {
		labels["toolchain.dev.openshift.com/type"] = nsType
	}
	return labels
}

func getNameWithTimestamp(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func newClient(t *testing.T) (*client.ApplyClient, *FakeClient) {
	cl := NewFakeClient(t)
	return client.NewApplyClient(cl), cl
}

func addToScheme(t *testing.T) *runtime.Scheme {
	s := scheme.Scheme
	err := authv1.Install(s)
	require.NoError(t, err)
	err = toolchainv1alpha1.AddToScheme(s)
	require.NoError(t, err)
	err = templatev1.Install(s)
	require.NoError(t, err)
	return s
}
