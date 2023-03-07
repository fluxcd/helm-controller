/*
Copyright 2022 The Flux authors

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

package kube

import (
	"github.com/fluxcd/pkg/runtime/client"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kubeCfg = `apiVersion: v1
kind: Config
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: https://1.2.3.4
  name: development
contexts:
- context:
    cluster: development
    namespace: frontend
    user: developer
  name: dev-frontend
current-context: dev-frontend
preferences: {}
users:
- name: developer
  user:
    password: some-password
    username: exp`
)

func TestConfigFromSecret(t *testing.T) {
	t.Run("with default key", func(t *testing.T) {
		g := NewWithT(t)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "super-secret",
				Namespace: "vault",
			},
			Data: map[string][]byte{
				DefaultKubeConfigSecretKey: []byte(kubeCfg),
				// Also confirm priority.
				DefaultKubeConfigSecretKeyExt: []byte("bad"),
			},
		}
		got, err := ConfigFromSecret(secret, "", client.KubeConfigOptions{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
	})

	t.Run("with default key with ext", func(t *testing.T) {
		g := NewWithT(t)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "super-secret",
				Namespace: "vault",
			},
			Data: map[string][]byte{
				DefaultKubeConfigSecretKeyExt: []byte(kubeCfg),
			},
		}
		got, err := ConfigFromSecret(secret, "", client.KubeConfigOptions{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
	})

	t.Run("with key", func(t *testing.T) {
		g := NewWithT(t)

		key := "cola.recipe"
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "super-secret",
				Namespace: "vault",
			},
			Data: map[string][]byte{
				key: []byte(kubeCfg),
			},
		}
		got, err := ConfigFromSecret(secret, key, client.KubeConfigOptions{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
	})

	t.Run("invalid key", func(t *testing.T) {
		g := NewWithT(t)

		key := "black-hole"
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "super-secret",
				Namespace: "vault",
			},
			Data: map[string][]byte{},
		}
		got, err := ConfigFromSecret(secret, key, client.KubeConfigOptions{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(BeNil())
		g.Expect(err.Error()).To(ContainSubstring("secret 'vault/super-secret' does not contain a 'black-hole' key "))
	})

	t.Run("key without data", func(t *testing.T) {
		g := NewWithT(t)

		key := "void"
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "super-secret",
				Namespace: "vault",
			},
			Data: map[string][]byte{
				key: nil,
			},
		}
		got, err := ConfigFromSecret(secret, key, client.KubeConfigOptions{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(BeNil())
		g.Expect(err.Error()).To(ContainSubstring("does not contain a 'void' key with data"))
	})

	t.Run("no keys", func(t *testing.T) {
		g := NewWithT(t)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "super-secret",
				Namespace: "vault",
			},
			Data: map[string][]byte{},
		}

		got, err := ConfigFromSecret(secret, "", client.KubeConfigOptions{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(BeNil())
		g.Expect(err.Error()).To(ContainSubstring("does not contain a 'value' or 'value.yaml'"))
	})

	t.Run("nil secret", func(t *testing.T) {
		g := NewWithT(t)

		got, err := ConfigFromSecret(nil, "", client.KubeConfigOptions{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(BeNil())
		g.Expect(err.Error()).To(ContainSubstring("secret is nil"))
	})

	t.Run("invalid kubeconfig", func(t *testing.T) {
		g := NewWithT(t)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "super-secret",
				Namespace: "vault",
			},
			Data: map[string][]byte{
				DefaultKubeConfigSecretKeyExt: []byte("bad"),
			},
		}

		got, err := ConfigFromSecret(secret, "", client.KubeConfigOptions{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(BeNil())
		g.Expect(err.Error()).To(ContainSubstring("couldn't get version/kind"))
	})

	t.Run("with kubeconfig options", func(t *testing.T) {
		g := NewWithT(t)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "super-secret",
				Namespace: "vault",
			},
			Data: map[string][]byte{
				DefaultKubeConfigSecretKey: []byte(kubeCfg),
			},
		}
		got, err := ConfigFromSecret(secret, "", client.KubeConfigOptions{
			UserAgent: "test",
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.UserAgent).To(Equal("test"))
	})
}
