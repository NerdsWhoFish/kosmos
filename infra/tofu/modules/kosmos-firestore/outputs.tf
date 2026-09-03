output "database_name" {
  value       = google_firestore_database.kosmos.name
  description = "Firestore database name."
}

output "database_location" {
  value       = google_firestore_database.kosmos.location_id
  description = "Firestore database location."
}
