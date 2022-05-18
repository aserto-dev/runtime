package mycars.GET.car

default allowed = false
default visible = false
default enabled = false

allowed {
  input.role == "admin"
}

enabled {
  input.role == "admin"
  input.role == "user"
}

visible {
  input.role == "admin"
  input.role == "user"
  input.role == "viewer"
}
