# services/

Business logic layer. Services orchestrate db queries (sqlc) and utils (OTP, hashing, sessions). Handlers call services; services should not depend on Echo. Return typed `model` structs and `*model.ApiError`.
