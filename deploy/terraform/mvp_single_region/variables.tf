variable "profile_id" {
  description = "Deployment profile identifier."
  type        = string
  default     = "mvp-single-region"
}

variable "rollout_path" {
  description = "Path to rollout config used by downstream deployment tooling."
  type        = string
  default     = "deploy/profiles/mvp-single-region/rollout.json"
}
