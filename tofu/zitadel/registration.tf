# The registration webhook (kubernetes/components/zitadel/registration-webhook.yaml): Zitadel
# calls it whenever a human user is created, and it gives the user the roles their organisation's
# metadata `registration-roles` names (project_arepazo.tf). Asynchronous - creating a user never
# waits for it - and called once; the webhook catches up on what it missed by itself. Like the
# groups target, the endpoint must pass the deny list, so the webhook is deployed first.
resource "zitadel_action_target" "registration" {
  name        = "registration-roles"
  endpoint    = "http://zitadel-registration.zitadel.svc"
  target_type = "REST_WEBHOOK"
  timeout     = "10s"
  # Only one target per event; there is nothing after it to stop.
  interrupt_on_error = false
}

# The webhook's user. It lives in Arepazo, where it was first needed, but serves any organisation
# that makes it an ORG_USER_MANAGER (zitadel_org_member, e.g. project_arepazo.tf) - Zitadel takes
# members from any organisation. Its personal access token is made in the console, so it never
# lands in this state (README "Adding a WordPress site", step 5).
resource "zitadel_machine_user" "registration" {
  org_id            = zitadel_organization.arepazo.id
  user_name         = "registration"
  name              = "Registration webhook"
  access_token_type = "ACCESS_TOKEN_TYPE_BEARER"
}

moved {
  from = zitadel_machine_user.arepazo_registration
  to   = zitadel_machine_user.registration
}

# Users created through the v2 API - the login page's registration, the console - and the
# older self-registration event.
resource "zitadel_action_execution_event" "registration" {
  for_each = toset(["user.human.added", "user.human.selfregistered"])

  event      = each.key
  target_ids = [zitadel_action_target.registration.id]
}
