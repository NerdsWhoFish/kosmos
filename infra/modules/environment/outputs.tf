output "project_id" {
  value = var.project_id
}

output "environment" {
  value = var.environment
}

output "region" {
  value = var.region
}

output "name_prefix" {
  value = local.name_prefix
}

output "deploy_service" {
  value = local.deploy_service
}

output "service_url" {
  value       = try(google_cloud_run_v2_service.kosmos[0].uri, null)
  description = "Cloud Run origin URL."
}

output "runtime_service_account" {
  value       = google_service_account.runtime.email
  description = "Cloud Run runtime service account."
}

output "firestore_database" {
  value       = google_firestore_database.kosmos.name
  description = "Firestore database name."
}

output "attachments_bucket" {
  value       = google_storage_bucket.attachments.name
  description = "Private attachments and receipts bucket."
}

output "artifact_registry_docker_prefix" {
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.kosmos.repository_id}"
  description = "Artifact Registry Docker repository prefix."
}

output "release_wif_provider" {
  value       = google_iam_workload_identity_pool_provider.github.name
  description = "GitHub OIDC provider for the Quill release workflow."
}

output "release_service_account" {
  value       = google_service_account.releaser.email
  description = "Service account the Quill release workflow impersonates."
}

output "release_image" {
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.kosmos.repository_id}/kosmos"
  description = "Artifact Registry image name for Quill releases."
}

output "secret_ids" {
  value       = { for key, secret in google_secret_manager_secret.runtime : key => secret.secret_id }
  description = "Secret Manager containers whose payloads are populated outside Git."
}
