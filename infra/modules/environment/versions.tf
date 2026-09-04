terraform {
  required_version = ">= 1.11.5"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.44.0, < 8.0.0"
    }
    grafana = {
      source  = "grafana/grafana"
      version = ">= 4.45.2, < 5.0.0"
    }
  }
}
