# The flat `groups` claim: before Zitadel issues an ID token, userinfo or a JWT access token, it
# calls the webhook in kubernetes/components/zitadel/groups-webhook.yaml and appends the list
# of role keys it answers. Zitadel checks the endpoint against its deny list when the target is
# created, so the webhook and the deny list exception must be deployed before this is applied.
resource "zitadel_action_target" "groups" {
  name        = "groups-claim"
  endpoint    = "http://zitadel-groups.zitadel.svc"
  target_type = "REST_CALL"
  timeout     = "5s"
  # Tokens without the claim rather than no login at all; the UIs then refuse the user.
  interrupt_on_error = false
}

resource "zitadel_action_execution_function" "groups" {
  for_each = toset(["preuserinfo", "preaccesstoken"])

  name       = each.key
  target_ids = [zitadel_action_target.groups.id]
}
