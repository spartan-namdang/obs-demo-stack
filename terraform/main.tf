resource "kubernetes_namespace" "argocd" {
  metadata {
    name = "argocd"
  }
}

resource "helm_release" "argocd" {
  name       = "argocd"
  repository = "https://argoproj.github.io/argo-helm"
  chart      = "argo-cd"
  version    = "9.1.0"
  namespace  = kubernetes_namespace.argocd.metadata[0].name

  # Enable insecure mode for the UI (simplifies local demo access)
  set {
    name  = "configs.params.server\\.insecure"
    value = "true"
  }
}