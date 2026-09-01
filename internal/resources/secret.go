/*
Copyright 2026 Paperclip Inc.

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

package resources

import (
	"crypto/sha1" // #nosec G505 -- htpasswd {SHA} format requires SHA-1; this is not a security-sensitive use
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// HtpasswdEntry returns a single htpasswd line in {SHA} format for the given username and password.
// {SHA} uses base64-encoded SHA-1 and is widely supported by nginx-ingress and other ingress controllers.
func HtpasswdEntry(username, password string) string {
	// #nosec G401 -- htpasswd {SHA} format requires SHA-1
	h := sha1.New()
	h.Write([]byte(password))
	digest := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("%s:{SHA}%s", username, digest)
}

// BuildBasicAuthSecret creates a Secret containing htpasswd content for Ingress Basic Authentication.
// The Secret holds three keys:
//   - "auth": htpasswd-formatted line (used by ingress controllers)
//   - "username": plaintext username
//   - "password": plaintext password
//
// The plaintext keys allow users to retrieve the auto-generated credentials,
// since the hashed htpasswd value in "auth" cannot be reversed.
func BuildBasicAuthSecret(instance *openclawv1alpha1.OpenClawInstance, password string) *corev1.Secret {
	username := AppName
	if instance.Spec.Networking.Ingress.Security.BasicAuth != nil &&
		instance.Spec.Networking.Ingress.Security.BasicAuth.Username != "" {
		username = instance.Spec.Networking.Ingress.Security.BasicAuth.Username
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      BasicAuthSecretName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
		Data: map[string][]byte{
			"auth":     []byte(HtpasswdEntry(username, password)),
			"username": []byte(username),
			"password": []byte(password),
		},
	}
}

// BuildGatewayTokenSecret creates a Secret containing the gateway authentication token.
// The token is used to configure gateway.auth.mode=token for shared-secret
// gateway authentication. Control UI device identity remains a separate check.
func BuildGatewayTokenSecret(instance *openclawv1alpha1.OpenClawInstance, tokenHex string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GatewayTokenSecretName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
		Data: map[string][]byte{
			GatewayTokenSecretKey: []byte(tokenHex),
		},
	}
}

// BuildMeshStateSecret creates an empty Secret for a mesh provider to persist
// node identity and certificate state across pod restarts. Tailscale's
// containerboot reads and writes it via the Kubernetes API when TS_KUBE_SECRET
// is set, which prevents hostname incrementing and Let's Encrypt certificate
// re-issuance on every restart.
func BuildMeshStateSecret(instance *openclawv1alpha1.OpenClawInstance, name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
	}
}

// BuildTailscaleStateSecret is retained for the Tailscale-specific name.
func BuildTailscaleStateSecret(instance *openclawv1alpha1.OpenClawInstance) *corev1.Secret {
	return BuildMeshStateSecret(instance, TailscaleStateSecretName(instance))
}
