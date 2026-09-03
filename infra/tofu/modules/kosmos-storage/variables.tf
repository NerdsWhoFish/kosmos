variable "project_id" {
  type        = string
  description = "GCP project that owns the attachment bucket."
}

variable "location" {
  type        = string
  description = "Bucket location."
  default     = "US-EAST1"
}

variable "bucket_name" {
  type        = string
  description = "Globally unique bucket name for private attachments and receipts."
}

variable "retention_days" {
  type        = number
  description = "Minimum retention period for uploaded files."
  default     = 30
}
