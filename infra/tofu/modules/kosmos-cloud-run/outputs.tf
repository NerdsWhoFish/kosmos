output "service_name" {
  value       = try(google_cloud_run_v2_service.kosmos[0].name, null)
  description = "Cloud Run service name."
}

output "service_uri" {
  value       = try(google_cloud_run_v2_service.kosmos[0].uri, null)
  description = "Cloud Run origin URI for Cloudflare or another edge layer."
}

output "runtime_service_account" {
  value       = google_service_account.runtime.email
  description = "Runtime service account email."
}

output "artifact_registry_repository" {
  value       = google_artifact_registry_repository.kosmos.name
  description = "Artifact Registry repository resource name."
}

output "release_wif_provider" {
  value       = google_iam_workload_identity_pool_provider.github.name
  description = "GitHub OIDC provider for Quill releases."
}

output "release_service_account" {
  value       = google_service_account.releaser.email
  description = "Service account for Quill Artifact Registry publishing."
}

output "release_image" {
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.kosmos.repository_id}/kosmos"
  description = "Artifact Registry image name for the Quill release workflow."
}
