# Import format: <project_id>:<token_id>
# Note: the `secret` attribute is UNKNOWN after import — Hetzner does not
# return it on subsequent reads. Any downstream provider that consumes the
# secret will need a fresh token created via terraform.
terraform import hcloudplus_project_token.rw 14138093:177763691
