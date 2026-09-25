# Lets pods log in with their service account token. OpenBao runs in the same cluster, so it
# verifies those tokens with its own service account's token and CA, which Kubernetes mounts
# into its pod; the chart's auth-delegator ClusterRoleBinding allows the TokenReview calls.
resource "vault_auth_backend" "kubernetes" {
  type = "kubernetes"
}

resource "vault_kubernetes_auth_backend_config" "kubernetes" {
  backend         = vault_auth_backend.kubernetes.path
  kubernetes_host = "https://kubernetes.default.svc"
}
