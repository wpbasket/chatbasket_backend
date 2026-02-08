# handler/

HTTP layer (Echo handlers). Keep logic minimal: validate/parse input, call services, map responses/cookies, and return typed payloads (`model`). No SQL or core business logic here.
