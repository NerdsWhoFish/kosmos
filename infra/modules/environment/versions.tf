terraform {
  required_version = ">= 1.11.5"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.44.0, < 8.0.0"
    }
  }
}
