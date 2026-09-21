# Two accounts, so both providers are aliased and there is deliberately no default.
# Every resource has to name its provider, which makes applying something to the wrong
# account a compile error rather than a surprise.
#
# The provider's own CLOUDFLARE_API_TOKEN environment variable is intentionally not
# used: with two accounts a single implicit token would silently authenticate against
# whichever one it belongs to.
provider "cloudflare" {
  alias     = "personal"
  api_token = var.cloudflare_token_personal
}

provider "cloudflare" {
  alias     = "arepazo"
  api_token = var.cloudflare_token_arepazo
}
