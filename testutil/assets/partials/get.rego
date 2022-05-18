package partials

default allowed = false

allowed_actions = [
    {"action": "view", "roles": ["admin", "member", "viewer"]},
    {"action": "add", "roles": ["admin", "member"]},
    {"action": "delete", "roles": ["admin"]},
]

allowed {
    op = allowed_actions[_]
    input.action = op.action
    some i
      input.role = op.roles[i]
}
