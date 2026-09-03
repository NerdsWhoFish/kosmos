output "service_name" {
  value       = google_cloud_run_v2_service.kosmos.name
  description = "Cloud Run service name."
}

output "service_uri" {
  value       = google_cloud_run_v2_service.kosmos.uri
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
