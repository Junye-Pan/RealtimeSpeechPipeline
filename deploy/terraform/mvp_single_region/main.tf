terraform {
  required_version = ">= 1.6.0"
}

provider "null" {}

resource "null_resource" "rspp_mvp_profile" {
  triggers = {
    profile_id   = var.profile_id
    rollout_path = var.rollout_path
  }
}
