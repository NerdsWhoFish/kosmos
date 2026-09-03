output "bucket_name" {
  value       = google_storage_bucket.attachments.name
  description = "Private attachment bucket name."
}

output "bucket_url" {
  value       = google_storage_bucket.attachments.url
  description = "Private attachment bucket URL."
}
