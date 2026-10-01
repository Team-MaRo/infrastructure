---
paths:
  - "tofu/**"
---

# OpenTofu

- Read `tofu/README.md` and the module's own `README.md` before changing a module; update the
  module's README in the same change.
- **Never run `tofu destroy`.**
- Never apply a plan that is not clean (`0 to add, 0 to change, 0 to destroy`). `forces
  replacement`, `must be replaced` or `will be destroyed` is a bug in the config - fix it and
  re-plan.
- The admin runs `tofu apply`. An agent may run `tofu fmt`, `tofu validate` and `tofu plan`;
  `tofu init -backend=false` needs no credentials.
- Adopting existing resources goes through `import` blocks in the module's `imports.tf`, never
  `tofu import`.
- No value belongs in a `.tf` file: tokens and addresses come from the gitignored
  `terraform.tfvars`, and every module ships a `terraform.tfvars.example`.
- Secrets never land in state: use write-only (`*_wo`) attributes and ephemeral resources
  where a secret has to pass through.
- Each module is its own root module with its own state key; none reads another's state.
