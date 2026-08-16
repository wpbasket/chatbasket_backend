# Backend Folder Structure

```
chatbasket_backend/
├── .agents/
│   ├── skills/
│   │   ├── building-native-ui/
│   │   │   ├── references/
│   │   │   │   ├── animations.md
│   │   │   │   ├── controls.md
│   │   │   │   ├── form-sheet.md
│   │   │   │   ├── gradients.md
│   │   │   │   ├── icons.md
│   │   │   │   ├── media.md
│   │   │   │   ├── route-structure.md
│   │   │   │   ├── search.md
│   │   │   │   ├── storage.md
│   │   │   │   ├── tabs.md
│   │   │   │   ├── toolbar-and-headers.md
│   │   │   │   ├── visual-effects.md
│   │   │   │   ├── webgpu-three.md
│   │   │   │   └── zoom-transitions.md
│   │   │   └── SKILL.md
│   │   ├── eas-update-insights/
│   │   │   ├── references/
│   │   │   │   ├── channel-insights-schema.md
│   │   │   │   └── update-insights-schema.md
│   │   │   └── SKILL.md
│   │   ├── expo-api-routes/
│   │   │   └── SKILL.md
│   │   ├── expo-cicd-workflows/
│   │   │   ├── scripts/
│   │   │   │   ├── fetch.js
│   │   │   │   ├── package.json
│   │   │   │   └── validate.js
│   │   │   └── SKILL.md
│   │   ├── expo-deployment/
│   │   │   ├── references/
│   │   │   │   ├── app-store-metadata.md
│   │   │   │   ├── ios-app-store.md
│   │   │   │   ├── play-store.md
│   │   │   │   ├── testflight.md
│   │   │   │   └── workflows.md
│   │   │   └── SKILL.md
│   │   ├── expo-dev-client/
│   │   │   └── SKILL.md
│   │   ├── expo-module/
│   │   │   ├── references/
│   │   │   │   ├── config-plugin.md
│   │   │   │   ├── create-expo-module.md
│   │   │   │   ├── lifecycle.md
│   │   │   │   ├── module-config.md
│   │   │   │   ├── native-module.md
│   │   │   │   └── native-view.md
│   │   │   └── SKILL.md
│   │   ├── expo-tailwind-setup/
│   │   │   └── SKILL.md
│   │   ├── expo-ui-jetpack-compose/
│   │   │   └── SKILL.md
│   │   ├── expo-ui-swiftui/
│   │   │   └── SKILL.md
│   │   ├── frontend-design/
│   │   │   ├── LICENSE.txt
│   │   │   └── SKILL.md
│   │   ├── native-data-fetching/
│   │   │   ├── references/
│   │   │   │   └── expo-router-loaders.md
│   │   │   └── SKILL.md
│   │   ├── ui-ux-pro-max/
│   │   │   ├── SKILL.md
│   │   │   ├── data
│   │   │   └── scripts
│   │   ├── upgrading-expo/
│   │   │   ├── references/
│   │   │   │   ├── expo-av-to-audio.md
│   │   │   │   ├── expo-av-to-video.md
│   │   │   │   ├── native-tabs.md
│   │   │   │   ├── new-architecture.md
│   │   │   │   ├── react-19.md
│   │   │   │   ├── react-compiler.md
│   │   │   │   └── react-navigation-to-expo-router.md
│   │   │   └── SKILL.md
│   │   └── use-dom/
│   │       └── SKILL.md
│   └── workflows/
│       ├── folder-tree-sync.md
│       ├── gitnexus-backup-cb-docs-references.md
│       ├── gitnexus-chatbasket-backend.md
│       ├── gitnexus-chatbasket.md
│       ├── gitnexus-helper-cb-backend.md
│       ├── gitnexus-wiki-backup-cb-docs-references.md
│       ├── gitnexus-wiki-chatbasket-backend.md
│       ├── gitnexus-wiki-chatbasket.md
│       └── gitnexus-wiki-helper-cb-backend.md
├── .claude/
│   ├── skills/
│   │   └── gitnexus/
│   │       ├── gitnexus-cli/
│   │       │   └── SKILL.md
│   │       ├── gitnexus-debugging/
│   │       │   └── SKILL.md
│   │       ├── gitnexus-exploring/
│   │       │   └── SKILL.md
│   │       ├── gitnexus-guide/
│   │       │   └── SKILL.md
│   │       ├── gitnexus-impact-analysis/
│   │       │   └── SKILL.md
│   │       └── gitnexus-refactoring/
│   │           └── SKILL.md
│   └── settings.json
├── .github/
│   └── workflows/
│       ├── deploy_relay.yml
│       └── deploy_web.yml
├── .gitnexus/
│   ├── parse-cache/
│   │   ├── dcb8a3d4bbc1c2e7cdc42bc7ad8d7c6488b6d2ca9334b394999967c6fb604a92.json
│   │   └── index.json
│   ├── parsedfile-cache/
│   │   ├── dcb8a3d4bbc1c2e7cdc42bc7ad8d7c6488b6d2ca9334b394999967c6fb604a92/
│   │   │   ├── dcb8a3d4bbc1c2e7cdc42bc7ad8d7c6488b6d2ca9334b394999967c6fb604a92-w1-0.json
│   │   │   ├── dcb8a3d4bbc1c2e7cdc42bc7ad8d7c6488b6d2ca9334b394999967c6fb604a92-w1-1.json
│   │   │   └── dcb8a3d4bbc1c2e7cdc42bc7ad8d7c6488b6d2ca9334b394999967c6fb604a92-w1-2.json
│   │   └── index.json
│   ├── .gitignore
│   ├── gitnexus.json
│   ├── lbug
│   ├── meta.json
│   └── run.cjs
├── chatbasket-api/
│   ├── app/
│   │   └── main.go
│   ├── cmd/
│   │   └── migrate/
│   │       └── main.go
│   ├── db/
│   │   ├── common/
│   │   │   ├── migrations/
│   │   │   │   ├── 001_auth_init.down.sql
│   │   │   │   ├── 001_auth_init.up.sql
│   │   │   │   ├── 002_auth_rate_limiter.down.sql
│   │   │   │   ├── 002_auth_rate_limiter.up.sql
│   │   │   │   ├── 003_qr_login.down.sql
│   │   │   │   └── 003_qr_login.up.sql
│   │   │   └── queries/
│   │   │       ├── auth.sql
│   │   │       ├── e2ee.sql
│   │   │       ├── keys_revision.sql
│   │   │       └── qr_login.sql
│   │   ├── personal/
│   │   │   ├── migrations/
│   │   │   │   ├── 001_personal_profile_system.down.sql
│   │   │   │   ├── 001_personal_profile_system.up.sql
│   │   │   │   ├── 002_personal_contact_system.down.sql
│   │   │   │   ├── 002_personal_contact_system.up.sql
│   │   │   │   ├── 003_personal_block_system.down.sql
│   │   │   │   ├── 003_personal_block_system.up.sql
│   │   │   │   ├── 004_personal_restrictions_system.down.sql
│   │   │   │   ├── 004_personal_restrictions_system.up.sql
│   │   │   │   ├── 005_personal_token_system.down.sql
│   │   │   │   ├── 005_personal_token_system.up.sql
│   │   │   │   ├── 006_personal_chat_system.down.sql
│   │   │   │   ├── 006_personal_chat_system.up.sql
│   │   │   │   ├── 007_block_sync_actions_cleanup.down.sql
│   │   │   │   ├── 007_block_sync_actions_cleanup.up.sql
│   │   │   │   ├── 010_pending_uploads.down.sql
│   │   │   │   ├── 010_pending_uploads.up.sql
│   │   │   │   ├── 011_history_sync.down.sql
│   │   │   │   └── 011_history_sync.up.sql
│   │   │   └── queries/
│   │   │       ├── pending_uploads.sql
│   │   │       ├── personal_chat.sql
│   │   │       ├── personal_contacts.sql
│   │   │       └── personal_profile.sql
│   │   └── public/
│   │       └── queries/
│   │           └── placeholder.sql
│   ├── docs/
│   │   └── TIMESTAMP_STANDARDS.md
│   ├── gen/
│   │   └── proto/
│   │       ├── common/
│   │       │   ├── error/
│   │       │   │   └── errors.pb.go
│   │       │   └── model/
│   │       │       └── model.pb.go
│   │       ├── core/
│   │       │   ├── core_auth/
│   │       │   │   ├── rpc_core_authv1connect/
│   │       │   │   │   └── core_auth_api.connect.go
│   │       │   │   └── core_auth_api.pb.go
│   │       │   └── core_email/
│   │       │       ├── rpc_core_emailv1connect/
│   │       │       │   └── core_email_api.connect.go
│   │       │       └── core_email_api.pb.go
│   │       └── personal/
│   │           ├── personal_chat/
│   │           │   ├── rpc_personal_chatv1connect/
│   │           │   │   └── personal_chat_api.connect.go
│   │           │   └── personal_chat_api.pb.go
│   │           ├── personal_contact/
│   │           │   ├── rpc_personal_contactv1connect/
│   │           │   │   └── personal_contact_api.connect.go
│   │           │   └── personal_contact_api.pb.go
│   │           ├── personal_profile/
│   │           │   ├── rpc_personal_profilev1connect/
│   │           │   │   └── personal_profile_base_api.connect.go
│   │           │   └── personal_profile_base_api.pb.go
│   │           ├── personal_setting/
│   │           │   ├── rpc_personal_settingv1connect/
│   │           │   │   └── personal_setting_api.connect.go
│   │           │   └── personal_setting_api.pb.go
│   │           └── personal_sse/
│   │               ├── rpc_personal_ssev1connect/
│   │               │   └── personal_sse_api.connect.go
│   │               └── personal_sse_api.pb.go
│   ├── internal/
│   │   ├── handlers/
│   │   ├── modules/
│   │   │   ├── core/
│   │   │   │   ├── core_auth/
│   │   │   │   │   ├── internal/
│   │   │   │   │   │   └── core_auth_store/
│   │   │   │   │   │       ├── auth.sql.go
│   │   │   │   │   │       ├── db.go
│   │   │   │   │   │       ├── e2ee.sql.go
│   │   │   │   │   │       ├── keys_revision.sql.go
│   │   │   │   │   │       ├── models.go
│   │   │   │   │   │       ├── qr_login.sql.go
│   │   │   │   │   │       └── querier.go
│   │   │   │   │   ├── core_auth_api_common_http_handler.go
│   │   │   │   │   ├── core_auth_api_http_handler.go
│   │   │   │   │   ├── core_auth_api_qr_handler.go
│   │   │   │   │   ├── core_auth_api_qr_handler_test.go
│   │   │   │   │   ├── core_auth_api_routes.go
│   │   │   │   │   ├── core_auth_connect_handler.go
│   │   │   │   │   ├── core_auth_errors.go
│   │   │   │   │   ├── core_auth_kit_otp.go
│   │   │   │   │   ├── core_auth_kit_password.go
│   │   │   │   │   ├── core_auth_mdl.go
│   │   │   │   │   ├── core_auth_mdl_common.go
│   │   │   │   │   ├── core_auth_mdl_helpers.go
│   │   │   │   │   ├── core_auth_rate_limit_test.go
│   │   │   │   │   ├── core_auth_svc.go
│   │   │   │   │   ├── core_auth_svc_common.go
│   │   │   │   │   ├── core_auth_svc_e2ee.go
│   │   │   │   │   ├── core_auth_svc_e2ee_integration_test.go
│   │   │   │   │   ├── core_auth_svc_e2ee_test.go
│   │   │   │   │   ├── core_auth_svc_flows.go
│   │   │   │   │   ├── core_auth_svc_helpers.go
│   │   │   │   │   ├── core_auth_svc_middleware.go
│   │   │   │   │   ├── core_auth_svc_qr.go
│   │   │   │   │   ├── core_auth_svc_qr_integration_test.go
│   │   │   │   │   ├── core_auth_svc_qr_test.go
│   │   │   │   │   └── core_auth_svc_qr_ws.go
│   │   │   │   └── pending_uploads/
│   │   │   │       ├── internal/
│   │   │   │       │   └── pending_uploads_store/
│   │   │   │       │       ├── db.go
│   │   │   │       │       ├── models.go
│   │   │   │       │       ├── pending_uploads.sql.go
│   │   │   │       │       └── querier.go
│   │   │   │       ├── pending_uploads_cleanup.go
│   │   │   │       └── pending_uploads_svc.go
│   │   │   └── personal/
│   │   │       ├── personal_chat/
│   │   │       │   ├── internal/
│   │   │       │   │   └── personal_chat_store/
│   │   │       │   │       ├── db.go
│   │   │       │   │       ├── history_sync_integration_test.go
│   │   │       │   │       ├── models.go
│   │   │       │   │       ├── personal_chat.sql.go
│   │   │       │   │       └── querier.go
│   │   │       │   ├── cleanup_integration_test.go
│   │   │       │   ├── history_sync_api_http_handler.go
│   │   │       │   ├── history_sync_svc.go
│   │   │       │   ├── history_sync_svc_integration_test.go
│   │   │       │   ├── personal_chat_api_http_handler.go
│   │   │       │   ├── personal_chat_api_routes.go
│   │   │       │   ├── personal_chat_connect_handler.go
│   │   │       │   ├── personal_chat_errors.go
│   │   │       │   ├── personal_chat_mdl.go
│   │   │       │   ├── personal_chat_svc.go
│   │   │       │   ├── personal_chat_svc_cleanup.go
│   │   │       │   ├── personal_chat_svc_cleanup_mock_test.go
│   │   │       │   ├── personal_chat_svc_file.go
│   │   │       │   ├── personal_chat_svc_idempotency_test.go
│   │   │       │   ├── personal_chat_svc_revision_test.go
│   │   │       │   ├── personal_chat_svc_session_filter_mock_test.go
│   │   │       │   └── stale_keys_error_test.go
│   │   │       ├── personal_contact/
│   │   │       │   ├── internal/
│   │   │       │   │   └── personal_contact_store/
│   │   │       │   │       ├── db.go
│   │   │       │   │       ├── models.go
│   │   │       │   │       ├── personal_contacts.sql.go
│   │   │       │   │       └── querier.go
│   │   │       │   ├── personal_contact_api_http_handler.go
│   │   │       │   ├── personal_contact_api_routes.go
│   │   │       │   ├── personal_contact_block_status_test.go
│   │   │       │   ├── personal_contact_connect_handler.go
│   │   │       │   ├── personal_contact_delete_integration_test.go
│   │   │       │   ├── personal_contact_errors.go
│   │   │       │   ├── personal_contact_mdl.go
│   │   │       │   ├── personal_contact_request_integration_test.go
│   │   │       │   ├── personal_contact_svc.go
│   │   │       │   ├── personal_contact_svc_helpers.go
│   │   │       │   └── personal_contact_svc_mock_test.go
│   │   │       ├── personal_profile/
│   │   │       │   ├── internal/
│   │   │       │   │   └── personal_profile_store/
│   │   │       │   │       ├── db.go
│   │   │       │   │       ├── models.go
│   │   │       │   │       ├── personal_profile.sql.go
│   │   │       │   │       └── querier.go
│   │   │       │   ├── personal_profile_api_http_handler.go
│   │   │       │   ├── personal_profile_api_routes.go
│   │   │       │   ├── personal_profile_block_status_integration_test.go
│   │   │       │   ├── personal_profile_connect_handler.go
│   │   │       │   ├── personal_profile_constants.go
│   │   │       │   ├── personal_profile_errors.go
│   │   │       │   ├── personal_profile_mdl.go
│   │   │       │   ├── personal_profile_svc.go
│   │   │       │   ├── personal_profile_svc_e2ee_integration_test.go
│   │   │       │   ├── personal_profile_svc_helpers.go
│   │   │       │   └── personal_profile_svc_mock_test.go
│   │   │       ├── personal_setting/
│   │   │       │   ├── personal_setting_api_http_handler.go
│   │   │       │   ├── personal_setting_api_routes.go
│   │   │       │   ├── personal_setting_connect_handler.go
│   │   │       │   ├── personal_setting_errors.go
│   │   │       │   ├── personal_setting_mdl.go
│   │   │       │   └── personal_setting_svc.go
│   │   │       └── personal_sse/
│   │   │           ├── personal_sse_api_routes.go
│   │   │           ├── personal_sse_connect_handler.go
│   │   │           ├── personal_sse_manager.go
│   │   │           └── personal_sse_postgres_listener.go
│   │   └── platform/
│   │       ├── clients/
│   │       │   ├── cosmos.go
│   │       │   ├── email.go
│   │       │   ├── email_test.go
│   │       │   ├── firebase.go
│   │       │   ├── postgres.go
│   │       │   ├── r2.go
│   │       │   ├── r2_integration_test.go
│   │       │   ├── r2_test.go
│   │       │   └── secrets.go
│   │       ├── config/
│   │       │   └── config.go
│   │       ├── connect_sse/
│   │       │   ├── connect_sse.go
│   │       │   ├── connect_sse_test.go
│   │       │   └── race_required_test.go
│   │       ├── kit/
│   │       │   ├── concurrent_delete.go
│   │       │   ├── connect_rpc.go
│   │       │   ├── crypto.go
│   │       │   ├── errors.go
│   │       │   ├── errors_test.go
│   │       │   ├── models.go
│   │       │   ├── proto_helpers.go
│   │       │   └── utils.go
│   │       ├── logger/
│   │       │   └── logger.go
│   │       ├── middleware/
│   │       │   ├── auth_session_middleware.go
│   │       │   └── middleware.go
│   │       ├── router/
│   │       │   └── routes.go
│   │       └── services/
│   │           └── services.go
│   ├── proto/
│   │   ├── common/
│   │   │   ├── error/
│   │   │   │   └── errors.proto
│   │   │   └── model/
│   │   │       └── model.proto
│   │   ├── core/
│   │   │   ├── core_auth/
│   │   │   │   └── core_auth_api.proto
│   │   │   └── core_email/
│   │   │       └── core_email_api.proto
│   │   ├── personal/
│   │   │   ├── personal_chat/
│   │   │   │   └── personal_chat_api.proto
│   │   │   ├── personal_contact/
│   │   │   │   └── personal_contact_api.proto
│   │   │   ├── personal_profile/
│   │   │   │   └── personal_profile_base_api.proto
│   │   │   ├── personal_setting/
│   │   │   │   └── personal_setting_api.proto
│   │   │   └── personal_sse/
│   │   │       └── personal_sse_api.proto
│   │   └── README.md
│   ├── .env
│   ├── .gitignore
│   ├── Dockerfile
│   ├── buf.gen.yaml
│   ├── buf.yaml
│   ├── go.mod
│   ├── go.sum
│   └── sqlc.yaml
├── deployment/
│   ├── docker-compose.yml
│   └── nginx.conf
├── docs/
│   ├── business-rules/
│   │   └── personal/
│   │       ├── personal.chat-system.md
│   │       └── personal.profile&contact-system.md
│   └── testing/
│       └── pgx-testing.md
├── heroku-mail-relay/
│   ├── app/
│   │   ├── main.go
│   │   ├── main_test.go
│   │   └── security.go
│   ├── gen/
│   │   └── proto/
│   │       └── core/
│   │           └── core_email/
│   │               ├── rpc_core_emailv1connect/
│   │               │   └── core_email_api.connect.go
│   │               └── core_email_api.pb.go
│   ├── Dockerfile
│   ├── README.md
│   ├── buf.gen.yaml
│   ├── go.mod
│   └── go.sum
├── scripts/
├── .gitignore
├── .gitnexusignore
├── AGENTS.md
├── CLAUDE.md
└── README.md
```

# Frontend Folder Structure

```
chatbasket/
├── .agents/
│   ├── skills/
│   │   ├── building-native-ui/
│   │   │   ├── references/
│   │   │   │   ├── animations.md
│   │   │   │   ├── controls.md
│   │   │   │   ├── form-sheet.md
│   │   │   │   ├── gradients.md
│   │   │   │   ├── icons.md
│   │   │   │   ├── media.md
│   │   │   │   ├── route-structure.md
│   │   │   │   ├── search.md
│   │   │   │   ├── storage.md
│   │   │   │   ├── tabs.md
│   │   │   │   ├── toolbar-and-headers.md
│   │   │   │   ├── visual-effects.md
│   │   │   │   ├── webgpu-three.md
│   │   │   │   └── zoom-transitions.md
│   │   │   └── SKILL.md
│   │   ├── eas-update-insights/
│   │   │   ├── references/
│   │   │   │   ├── channel-insights-schema.md
│   │   │   │   └── update-insights-schema.md
│   │   │   └── SKILL.md
│   │   ├── expo-api-routes/
│   │   │   └── SKILL.md
│   │   ├── expo-cicd-workflows/
│   │   │   ├── scripts/
│   │   │   │   ├── fetch.js
│   │   │   │   ├── package.json
│   │   │   │   └── validate.js
│   │   │   └── SKILL.md
│   │   ├── expo-deployment/
│   │   │   ├── references/
│   │   │   │   ├── app-store-metadata.md
│   │   │   │   ├── ios-app-store.md
│   │   │   │   ├── play-store.md
│   │   │   │   ├── testflight.md
│   │   │   │   └── workflows.md
│   │   │   └── SKILL.md
│   │   ├── expo-dev-client/
│   │   │   └── SKILL.md
│   │   ├── expo-module/
│   │   │   ├── references/
│   │   │   │   ├── config-plugin.md
│   │   │   │   ├── create-expo-module.md
│   │   │   │   ├── lifecycle.md
│   │   │   │   ├── module-config.md
│   │   │   │   ├── native-module.md
│   │   │   │   └── native-view.md
│   │   │   └── SKILL.md
│   │   ├── expo-tailwind-setup/
│   │   │   └── SKILL.md
│   │   ├── expo-ui-jetpack-compose/
│   │   │   └── SKILL.md
│   │   ├── expo-ui-swiftui/
│   │   │   └── SKILL.md
│   │   ├── frontend-design/
│   │   │   ├── LICENSE.txt
│   │   │   └── SKILL.md
│   │   ├── native-data-fetching/
│   │   │   ├── references/
│   │   │   │   └── expo-router-loaders.md
│   │   │   └── SKILL.md
│   │   ├── ui-ux-pro-max/
│   │   │   ├── SKILL.md
│   │   │   ├── data
│   │   │   └── scripts
│   │   ├── upgrading-expo/
│   │   │   ├── references/
│   │   │   │   ├── expo-av-to-audio.md
│   │   │   │   ├── expo-av-to-video.md
│   │   │   │   ├── native-tabs.md
│   │   │   │   ├── new-architecture.md
│   │   │   │   ├── react-19.md
│   │   │   │   ├── react-compiler.md
│   │   │   │   └── react-navigation-to-expo-router.md
│   │   │   └── SKILL.md
│   │   └── use-dom/
│   │       └── SKILL.md
│   └── workflows/
│       ├── folder-tree-sync.md
│       ├── gitnexus-backup-cb-docs-references.md
│       ├── gitnexus-chatbasket-backend.md
│       ├── gitnexus-chatbasket.md
│       ├── gitnexus-helper-cb-backend.md
│       ├── gitnexus-wiki-backup-cb-docs-references.md
│       ├── gitnexus-wiki-chatbasket-backend.md
│       ├── gitnexus-wiki-chatbasket.md
│       └── gitnexus-wiki-helper-cb-backend.md
├── .claude/
│   ├── skills/
│   │   ├── building-native-ui/
│   │   │   ├── references/
│   │   │   │   ├── animations.md
│   │   │   │   ├── controls.md
│   │   │   │   ├── form-sheet.md
│   │   │   │   ├── gradients.md
│   │   │   │   ├── icons.md
│   │   │   │   ├── media.md
│   │   │   │   ├── route-structure.md
│   │   │   │   ├── search.md
│   │   │   │   ├── storage.md
│   │   │   │   ├── tabs.md
│   │   │   │   ├── toolbar-and-headers.md
│   │   │   │   ├── visual-effects.md
│   │   │   │   ├── webgpu-three.md
│   │   │   │   └── zoom-transitions.md
│   │   │   └── SKILL.md
│   │   ├── eas-update-insights/
│   │   │   ├── references/
│   │   │   │   ├── channel-insights-schema.md
│   │   │   │   └── update-insights-schema.md
│   │   │   └── SKILL.md
│   │   ├── expo-api-routes/
│   │   │   └── SKILL.md
│   │   ├── expo-cicd-workflows/
│   │   │   ├── scripts/
│   │   │   │   ├── fetch.js
│   │   │   │   ├── package.json
│   │   │   │   └── validate.js
│   │   │   └── SKILL.md
│   │   ├── expo-deployment/
│   │   │   ├── references/
│   │   │   │   ├── app-store-metadata.md
│   │   │   │   ├── ios-app-store.md
│   │   │   │   ├── play-store.md
│   │   │   │   ├── testflight.md
│   │   │   │   └── workflows.md
│   │   │   └── SKILL.md
│   │   ├── expo-dev-client/
│   │   │   └── SKILL.md
│   │   ├── expo-module/
│   │   │   ├── references/
│   │   │   │   ├── config-plugin.md
│   │   │   │   ├── create-expo-module.md
│   │   │   │   ├── lifecycle.md
│   │   │   │   ├── module-config.md
│   │   │   │   ├── native-module.md
│   │   │   │   └── native-view.md
│   │   │   └── SKILL.md
│   │   ├── expo-tailwind-setup/
│   │   │   └── SKILL.md
│   │   ├── expo-ui-jetpack-compose/
│   │   │   └── SKILL.md
│   │   ├── expo-ui-swiftui/
│   │   │   └── SKILL.md
│   │   ├── frontend-design/
│   │   │   ├── LICENSE.txt
│   │   │   └── SKILL.md
│   │   ├── gitnexus/
│   │   │   ├── gitnexus-cli/
│   │   │   │   └── SKILL.md
│   │   │   ├── gitnexus-debugging/
│   │   │   │   └── SKILL.md
│   │   │   ├── gitnexus-exploring/
│   │   │   │   └── SKILL.md
│   │   │   ├── gitnexus-guide/
│   │   │   │   └── SKILL.md
│   │   │   ├── gitnexus-impact-analysis/
│   │   │   │   └── SKILL.md
│   │   │   └── gitnexus-refactoring/
│   │   │       └── SKILL.md
│   │   ├── native-data-fetching/
│   │   │   ├── references/
│   │   │   │   └── expo-router-loaders.md
│   │   │   └── SKILL.md
│   │   ├── ui-ux-pro-max/
│   │   │   ├── SKILL.md
│   │   │   ├── data
│   │   │   └── scripts
│   │   ├── upgrading-expo/
│   │   │   ├── references/
│   │   │   │   ├── expo-av-to-audio.md
│   │   │   │   ├── expo-av-to-video.md
│   │   │   │   ├── native-tabs.md
│   │   │   │   ├── new-architecture.md
│   │   │   │   ├── react-19.md
│   │   │   │   ├── react-compiler.md
│   │   │   │   └── react-navigation-to-expo-router.md
│   │   │   └── SKILL.md
│   │   └── use-dom/
│   │       └── SKILL.md
│   └── settings.json
├── .github/
│   └── workflows/
│       └── deploy-web.yml
├── .gitnexus/
│   ├── parse-cache/
│   │   ├── 0102a7757a6837bf097a4775cdc98296052827d7bb5b12140226d5fa2db314d4.json
│   │   └── index.json
│   ├── parsedfile-cache/
│   │   ├── 0102a7757a6837bf097a4775cdc98296052827d7bb5b12140226d5fa2db314d4/
│   │   │   ├── 0102a7757a6837bf097a4775cdc98296052827d7bb5b12140226d5fa2db314d4-w1-0.json
│   │   │   ├── 0102a7757a6837bf097a4775cdc98296052827d7bb5b12140226d5fa2db314d4-w1-1.json
│   │   │   └── 0102a7757a6837bf097a4775cdc98296052827d7bb5b12140226d5fa2db314d4-w1-2.json
│   │   └── index.json
│   ├── .gitignore
│   ├── gitnexus.json
│   ├── lbug
│   ├── meta.json
│   └── run.cjs
├── .vscode/
│   ├── extensions.json
│   └── settings.json
├── assets/
│   ├── data/
│   │   ├── followerRelations.ts
│   │   ├── posts.ts
│   │   └── users.ts
│   ├── expo.icon/
│   │   ├── Assets/
│   │   │   ├── expo-symbol 2.svg
│   │   │   └── grid.png
│   │   └── icon.json
│   ├── fonts/
│   │   ├── AstaSans-Regular.ttf
│   │   ├── AstaSans-SemiBold.ttf
│   │   ├── Gantari-ExtraLight.ttf
│   │   ├── Gantari-Regular.ttf
│   │   └── Gantari-SemiBold.ttf
│   └── images/
│       ├── adaptive-icon.png
│       ├── expo-logo.png
│       ├── favicon.png
│       ├── icon.png
│       ├── logo-glow.png
│       ├── partial-react-logo.png
│       ├── react-logo.png
│       ├── react-logo@2x.png
│       ├── react-logo@3x.png
│       └── splash-icon.png
├── coverage/
│   ├── lcov-report/
│   │   ├── base.css
│   │   ├── block-navigation.js
│   │   ├── chat.sse.subscriber.ts.html
│   │   ├── favicon.png
│   │   ├── index.html
│   │   ├── prettify.css
│   │   ├── prettify.js
│   │   ├── sort-arrow-sprite.png
│   │   └── sorter.js
│   ├── clover.xml
│   ├── coverage-final.json
│   └── lcov.info
├── docs/
│   ├── CHANGE_POLICY.md
│   ├── KEYBOARD_VIEW.md
│   ├── PROJECT_CONSISTENCY.md
│   └── SAVE_TO_DEVICE_ANDROID.md
├── modules/
│   └── save-to-device/
│       ├── android/
│       │   ├── src/
│       │   │   └── main/
│       │   │       ├── java/
│       │   │       │   └── expo/
│       │   │       │       └── modules/
│       │   │       │           └── savetodevice/
│       │   │       │               └── SaveToDeviceModule.kt
│       │   │       └── AndroidManifest.xml
│       │   └── build.gradle
│       ├── ios/
│       │   ├── SaveToDevice.podspec
│       │   └── SaveToDeviceModule.swift
│       ├── src/
│       │   ├── SaveToDevice.types.ts
│       │   ├── SaveToDeviceModule.ts
│       │   ├── SaveToDeviceModule.web.ts
│       │   └── SaveToDeviceView.web.tsx
│       ├── expo-module.config.json
│       └── index.ts
├── public/
│   ├── .well-known/
│   │   └── assetlinks.json
│   ├── .cloudflareignore
│   └── _headers
├── scripts/
│   ├── fix-cloudflare-pages.ts
│   ├── prune-pages-deployments.ts
│   └── verify-build.ts
├── src/
│   ├── __tests__/
│   │   ├── components/
│   │   │   └── privacy_avatar.test.ts
│   │   ├── lib/
│   │   │   ├── chat/
│   │   │   │   └── chat.recovery.test.ts
│   │   │   ├── clients/
│   │   │   │   └── connect_error_details.test.ts
│   │   │   ├── e2ee/
│   │   │   │   ├── e2ee_recipient_key_validation.test.ts
│   │   │   │   ├── e2ee_v3_download_pipeline.test.ts
│   │   │   │   ├── e2ee_v3_envelope.test.ts
│   │   │   │   └── e2ee_v3_service_multikey.test.ts
│   │   │   ├── outbox/
│   │   │   │   ├── outbox.live_server.probe.test.ts
│   │   │   │   └── outbox.preparing.test.ts
│   │   │   ├── sse/
│   │   │   │   ├── chat.sse.subscriber.e2ee_v3.test.ts
│   │   │   │   ├── chat.sse.subscriber.failures.test.ts
│   │   │   │   ├── chat.sse.subscriber.test.ts
│   │   │   │   ├── personal.api.sse.test.ts
│   │   │   │   ├── personal.sse.module.base.test.ts
│   │   │   │   └── sse.live_server.network_pause.probe.test.ts
│   │   │   └── personal.session.coordinator.recovery.test.ts
│   │   ├── state/
│   │   │   ├── chat/
│   │   │   │   ├── ack_race_condition.test.ts
│   │   │   │   ├── avatar_caching.test.ts
│   │   │   │   ├── history_sync.test.ts
│   │   │   │   ├── is_contactable.test.ts
│   │   │   │   ├── pending_preview_heal.test.ts
│   │   │   │   ├── sync_catchup_rerun.test.ts
│   │   │   │   └── ui_sort_local_seq.test.ts
│   │   │   ├── contacts/
│   │   │   │   └── personal.state.contacts.test.ts
│   │   │   ├── network/
│   │   │   │   └── state.network.online_verify.test.ts
│   │   │   ├── personalState/
│   │   │   │   └── profile_avatar.test.ts
│   │   │   └── userProfiles/
│   │   │       ├── user_profiles_appliers.test.ts
│   │   │       └── user_profiles_store.test.ts
│   │   ├── storage/
│   │   │   ├── auth.e2ee_revision.test.ts
│   │   │   ├── auth.logout_stream_stop.test.ts
│   │   │   ├── chat.storage.localSeq.test.ts
│   │   │   ├── sqlite_key.test.ts
│   │   │   └── storage_init.e2ee_seed.test.ts
│   │   └── utils/
│   │       ├── personalUtils/
│   │       │   ├── util.contactErrors.test.ts
│   │       │   └── util.messageTick.test.ts
│   │       └── personal_util_device.test.ts
│   ├── app/
│   │   ├── (auth)/
│   │   │   ├── auth-verify.tsx
│   │   │   ├── auth.tsx
│   │   │   └── index.tsx
│   │   ├── personal/
│   │   │   ├── contacts/
│   │   │   │   ├── blocks/
│   │   │   │   │   └── index.tsx
│   │   │   │   ├── chat/
│   │   │   │   │   ├── user/
│   │   │   │   │   │   └── [user_id].tsx
│   │   │   │   │   └── [chat_id].tsx
│   │   │   │   ├── components/
│   │   │   │   │   ├── ContactRow.tsx
│   │   │   │   │   ├── ContactsBulkActionBar.tsx
│   │   │   │   │   ├── ContactsHeaderSection.tsx
│   │   │   │   │   └── ContactsSegmentTabs.tsx
│   │   │   │   ├── requests/
│   │   │   │   │   ├── components/
│   │   │   │   │   │   ├── PendingRequestRow.tsx
│   │   │   │   │   │   ├── RequestsHeaderSummary.tsx
│   │   │   │   │   │   ├── RequestsSegmentTabs.tsx
│   │   │   │   │   │   └── SentRequestRow.tsx
│   │   │   │   │   ├── _layout.tsx
│   │   │   │   │   ├── index.tsx
│   │   │   │   │   ├── requests.flows.tsx
│   │   │   │   │   └── requests.styles.ts
│   │   │   │   ├── _layout.tsx
│   │   │   │   ├── contacts.flows.tsx
│   │   │   │   ├── contacts.styles.ts
│   │   │   │   └── index.tsx
│   │   │   ├── home/
│   │   │   │   ├── _components/
│   │   │   │   │   └── ChatListItem.tsx
│   │   │   │   ├── chat/
│   │   │   │   │   ├── _components/
│   │   │   │   │   │   ├── BulkActionBar.tsx
│   │   │   │   │   │   ├── ChatInputBar.tsx
│   │   │   │   │   │   └── MessageBubble.tsx
│   │   │   │   │   ├── user/
│   │   │   │   │   │   └── [user_id].tsx
│   │   │   │   │   ├── README_CHAT.md
│   │   │   │   │   ├── [chat_id].tsx
│   │   │   │   │   ├── _layout.tsx
│   │   │   │   │   └── index.tsx
│   │   │   │   ├── _layout.tsx
│   │   │   │   └── index.tsx
│   │   │   ├── profile/
│   │   │   │   ├── create-profile/
│   │   │   │   │   ├── components/
│   │   │   │   │   │   └── CreateProfileForm.tsx
│   │   │   │   │   ├── _layout.tsx
│   │   │   │   │   ├── create-profile.styles.ts
│   │   │   │   │   └── index.tsx
│   │   │   │   ├── settings/
│   │   │   │   │   ├── components/
│   │   │   │   │   │   ├── SettingsEmailRow.tsx
│   │   │   │   │   │   └── SettingsPasswordRow.tsx
│   │   │   │   │   ├── qr-login/
│   │   │   │   │   │   ├── _layout.tsx
│   │   │   │   │   │   └── index.tsx
│   │   │   │   │   ├── _layout.tsx
│   │   │   │   │   ├── index.tsx
│   │   │   │   │   ├── settings.flows.tsx
│   │   │   │   │   └── settings.styles.ts
│   │   │   │   ├── update-profile/
│   │   │   │   │   ├── components/
│   │   │   │   │   │   ├── UpdateProfileAvatarSection.tsx
│   │   │   │   │   │   └── UpdateProfileForm.tsx
│   │   │   │   │   ├── _layout.tsx
│   │   │   │   │   ├── index.tsx
│   │   │   │   │   └── update-profile.styles.ts
│   │   │   │   ├── _layout.tsx
│   │   │   │   ├── index.tsx
│   │   │   │   └── profile.styles.ts
│   │   │   ├── _layout.tsx
│   │   │   └── index.tsx
│   │   ├── public/
│   │   │   ├── explore/
│   │   │   │   ├── _layout.tsx
│   │   │   │   └── index.tsx
│   │   │   ├── home/
│   │   │   │   ├── post/
│   │   │   │   │   ├── _layout.tsx
│   │   │   │   │   └── index.tsx
│   │   │   │   ├── tempprofile/
│   │   │   │   │   ├── _layout.tsx
│   │   │   │   │   └── index.tsx
│   │   │   │   ├── _layout.tsx
│   │   │   │   └── index.tsx
│   │   │   ├── profile/
│   │   │   │   ├── create-profile/
│   │   │   │   │   ├── _layout.tsx
│   │   │   │   │   └── index.tsx
│   │   │   │   ├── settings/
│   │   │   │   │   ├── _layout.tsx
│   │   │   │   │   ├── index.tsx
│   │   │   │   │   └── settings.flows.tsx
│   │   │   │   ├── update-profile/
│   │   │   │   │   ├── _layout.tsx
│   │   │   │   │   └── index.tsx
│   │   │   │   ├── _layout.tsx
│   │   │   │   └── index.tsx
│   │   │   ├── _layout.tsx
│   │   │   └── index.tsx
│   │   ├── +native-intent.tsx
│   │   ├── +not-found.tsx
│   │   ├── README_ROOT_ARCHITECTURE.md
│   │   ├── _layout.tsx
│   │   ├── index.tsx
│   │   └── stream-test-manager.tsx
│   ├── components/
│   │   ├── header/
│   │   │   └── Header.tsx
│   │   ├── modals/
│   │   │   ├── types/
│   │   │   │   ├── AlertModal.tsx
│   │   │   │   ├── ConfirmModal.tsx
│   │   │   │   ├── ControllersModal.tsx
│   │   │   │   ├── DropdownPickerModal.tsx
│   │   │   │   ├── LoadingModal.tsx
│   │   │   │   └── modal.types.ts
│   │   │   ├── AppModal.tsx
│   │   │   └── README_MODAL_ARCHITECTURE.md
│   │   ├── personal/
│   │   │   ├── common/
│   │   │   │   └── PrivacyAvatar.tsx
│   │   │   └── profile/
│   │   │       └── ProfileAvatar.tsx
│   │   ├── publicComponents/
│   │   │   ├── post/
│   │   │   │   └── Postcard.tsx
│   │   │   └── profile/
│   │   │       ├── sections/
│   │   │       │   ├── TabsSection.tsx
│   │   │       │   └── UserInfoSection.tsx
│   │   │       ├── FollowerCard.tsx
│   │   │       └── ProfileList.tsx
│   │   ├── sidebar/
│   │   │   ├── Sidebar.tsx
│   │   │   └── VerticalTabBar.tsx
│   │   ├── tools/
│   │   │   └── KeyboardSync.tsx
│   │   ├── ui/
│   │   │   ├── common/
│   │   │   │   ├── AppButton.tsx
│   │   │   │   ├── Collapsible.tsx
│   │   │   │   ├── DropDown.tsx
│   │   │   │   ├── EmptyState.tsx
│   │   │   │   ├── HapticTab.tsx
│   │   │   │   ├── HelloWave.tsx
│   │   │   │   ├── LoadingScreen.tsx
│   │   │   │   ├── LoginPrompt.tsx
│   │   │   │   ├── ParallaxScrollView.tsx
│   │   │   │   ├── TabBarBackground.ios.tsx
│   │   │   │   ├── TabBarBackground.tsx
│   │   │   │   ├── TabBarButton.tsx
│   │   │   │   ├── ThemedText.tsx
│   │   │   │   ├── ThemedView.tsx
│   │   │   │   ├── ThemedViewWithSidebar.tsx
│   │   │   │   ├── UsernameDisplay.tsx
│   │   │   │   ├── animated-icon.module.css
│   │   │   │   ├── animated-icon.tsx
│   │   │   │   ├── animated-icon.web.tsx
│   │   │   │   ├── external-link.tsx
│   │   │   │   └── hint-row.tsx
│   │   │   ├── fonts/
│   │   │   │   ├── IconSymbol.tsx
│   │   │   │   ├── entypoIcons.tsx
│   │   │   │   ├── fontAwesome5.tsx
│   │   │   │   └── materialCommunityIcons.tsx
│   │   │   ├── README_UI_SYSTEM.md
│   │   │   └── basic.tsx
│   │   ├── personal.app.tabs.tsx
│   │   ├── personal.app.tabs.web.tsx
│   │   ├── public.app.tabs.tsx
│   │   └── public.app.tabs.web.tsx
│   ├── constants/
│   │   ├── Colors.ts
│   │   ├── fonts.ts
│   │   └── theme.ts
│   ├── gen/
│   │   └── proto/
│   │       ├── common/
│   │       │   ├── error/
│   │       │   │   └── errors_pb.ts
│   │       │   └── model/
│   │       │       └── model_pb.ts
│   │       ├── core/
│   │       │   └── core_auth/
│   │       │       └── core_auth_api_pb.ts
│   │       └── personal/
│   │           ├── personal_chat/
│   │           │   └── personal_chat_api_pb.ts
│   │           ├── personal_contact/
│   │           │   └── personal_contact_api_pb.ts
│   │           ├── personal_profile/
│   │           │   └── personal_profile_base_api_pb.ts
│   │           ├── personal_setting/
│   │           │   └── personal_setting_api_pb.ts
│   │           └── personal_sse/
│   │               └── personal_sse_api_pb.ts
│   ├── hooks/
│   │   ├── commonHooks/
│   │   │   ├── hooks.notificationPermission.ts
│   │   │   ├── hooks.notificationPermission.web.ts
│   │   │   ├── hooks.pressableAnimation.ts
│   │   │   ├── hooks.qrLogin.ts
│   │   │   ├── hooks.qrScanner.ts
│   │   │   └── hooks.theme.ts
│   │   ├── personalHooks/
│   │   └── useIncomingShare.ts
│   ├── lib/
│   │   ├── commonLib/
│   │   │   ├── authApi/
│   │   │   │   └── common.api.auth.ts
│   │   │   ├── models/
│   │   │   │   └── common.model.setting.ts
│   │   │   ├── settingApi/
│   │   │   │   └── common.api.setting.ts
│   │   │   └── index.ts
│   │   ├── constantLib/
│   │   │   ├── authApi/
│   │   │   │   └── api.auth.ts
│   │   │   ├── clients/
│   │   │   │   ├── client.ts
│   │   │   │   ├── connectClient.ts
│   │   │   │   ├── errorDetailRegistry.ts
│   │   │   │   ├── fileClient.ts
│   │   │   │   └── fileClient.web.ts
│   │   │   ├── constants/
│   │   │   │   └── constants.ts
│   │   │   ├── models/
│   │   │   │   ├── model.api.ts
│   │   │   │   └── model.auth.ts
│   │   │   └── index.ts
│   │   ├── personalLib/
│   │   │   ├── chatApi/
│   │   │   │   ├── chat.recovery.ts
│   │   │   │   ├── chat.sse.subscriber.ts
│   │   │   │   ├── chat.transport.ts
│   │   │   │   ├── connection.watcher.ts
│   │   │   │   ├── history.sync.ts
│   │   │   │   ├── outbox.errors.ts
│   │   │   │   ├── outbox.queue.ts
│   │   │   │   └── personal.api.chat.ts
│   │   │   ├── constant/
│   │   │   │   └── constant.chat.ts
│   │   │   ├── contactApi/
│   │   │   │   ├── personal.api.contact.ts
│   │   │   │   └── personal.api.error.block.ts
│   │   │   ├── e2ee/
│   │   │   │   ├── e2ee.crypto.ts
│   │   │   │   ├── e2ee.keys.ts
│   │   │   │   ├── e2ee.log.ts
│   │   │   │   ├── e2ee.service.ts
│   │   │   │   └── index.ts
│   │   │   ├── fileSystem/
│   │   │   │   ├── file.copy.ts
│   │   │   │   ├── file.download.ts
│   │   │   │   └── file.upload.ts
│   │   │   ├── models/
│   │   │   │   ├── personal.model.chat.ts
│   │   │   │   ├── personal.model.contact.ts
│   │   │   │   ├── personal.model.notification.ts
│   │   │   │   ├── personal.model.profile.ts
│   │   │   │   └── personal.model.setting.ts
│   │   │   ├── profileApi/
│   │   │   │   ├── personal.api.profile.ts
│   │   │   │   └── profile.service.ts
│   │   │   ├── settingApi/
│   │   │   │   └── personal.api.setting.ts
│   │   │   ├── sseApi/
│   │   │   │   ├── personal.api.sse.ts
│   │   │   │   └── personal.sse.module.base.ts
│   │   │   ├── index.ts
│   │   │   └── personal.session.coordinator.ts
│   │   ├── publicLib/
│   │   │   ├── models/
│   │   │   │   ├── public.model.profile.ts
│   │   │   │   └── public.model.setting.ts
│   │   │   ├── profileApi/
│   │   │   │   └── public.api.profile.ts
│   │   │   ├── settingApi/
│   │   │   │   └── public.api.setting.ts
│   │   │   └── index.ts
│   │   ├── storage/
│   │   │   ├── commonStorage/
│   │   │   │   ├── storage.auth.ts
│   │   │   │   ├── storage.preferences.ts
│   │   │   │   └── storage.sqliteKey.ts
│   │   │   ├── personalStorage/
│   │   │   │   ├── chat/
│   │   │   │   │   ├── chat.storage.native.ts
│   │   │   │   │   ├── chat.storage.normalize.ts
│   │   │   │   │   ├── chat.storage.schema.ts
│   │   │   │   │   ├── chat.storage.ts
│   │   │   │   │   ├── chat.storage.web.ts
│   │   │   │   │   └── personal.storage.chat.ts
│   │   │   │   ├── profile/
│   │   │   │   │   ├── personal.storage.user.ts
│   │   │   │   │   └── profile.storage.ts
│   │   │   │   ├── personal.storage.contacts.ts
│   │   │   │   └── personal.storage.device.ts
│   │   │   ├── README_STORAGE.md
│   │   │   ├── storage.init.ts
│   │   │   └── storage.wrapper.ts
│   │   └── README_API_ARCHITECTURE.md
│   ├── model/
│   │   ├── Follow.ts
│   │   ├── Post.ts
│   │   └── User.ts
│   ├── notification/
│   │   ├── README.md
│   │   ├── README_NOTIFICATIONS.md
│   │   ├── registerFcmOrApn.ts
│   │   └── registerFcmOrApn.web.ts
│   ├── state/
│   │   ├── appMode/
│   │   │   ├── README_DEEP_LINKING.md
│   │   │   └── state.appMode.ts
│   │   ├── auth/
│   │   │   ├── README_AUTH.md
│   │   │   ├── state.auth.forgotPassword.ts
│   │   │   ├── state.auth.loginOrSignup.ts
│   │   │   ├── state.auth.qrScanner.ts
│   │   │   └── state.auth.ts
│   │   ├── modals/
│   │   │   └── state.modals.ts
│   │   ├── personalState/
│   │   │   ├── blocks/
│   │   │   │   └── personal.state.blocks.ts
│   │   │   ├── chat/
│   │   │   │   ├── personal.state.chat.ts
│   │   │   │   └── personal.state.sync.ts
│   │   │   ├── contacts/
│   │   │   │   └── personal.state.contacts.ts
│   │   │   ├── home/
│   │   │   │   └── personal.state.home.ts
│   │   │   ├── profile/
│   │   │   │   ├── personal.state.profile.createProfile.ts
│   │   │   │   └── personal.state.profile.updateProfile.ts
│   │   │   ├── sse/
│   │   │   │   └── personal.state.sse.ts
│   │   │   ├── user/
│   │   │   │   └── personal.state.user.ts
│   │   │   └── userProfiles/
│   │   │       └── personal.state.userProfiles.ts
│   │   ├── publicState/
│   │   │   ├── profile/
│   │   │   │   ├── public.state.profile.createProfile.ts
│   │   │   │   └── public.state.profile.updateProfile.ts
│   │   │   ├── public.state.activePost.ts
│   │   │   ├── public.state.activeUser.ts
│   │   │   ├── public.state.activeUserFollowers.ts
│   │   │   ├── public.state.activeUserFollowing.ts
│   │   │   ├── public.state.activeUserPosts.ts
│   │   │   ├── public.state.initUserPosts.ts
│   │   │   └── public.state.userPostsStore.ts
│   │   ├── settings/
│   │   │   └── state.setting.ts
│   │   ├── theme/
│   │   │   ├── README_THEME.md
│   │   │   └── state.theme.ts
│   │   ├── tools/
│   │   │   ├── state.appState.ts
│   │   │   └── state.network.ts
│   │   ├── ui/
│   │   │   └── state.ui.ts
│   │   └── README_STATE_PATTERNS.md
│   ├── utils/
│   │   ├── commonUtils/
│   │   │   ├── util.date.ts
│   │   │   ├── util.error.ts
│   │   │   ├── util.modal.ts
│   │   │   ├── util.resendCooldown.ts
│   │   │   ├── util.router.ts
│   │   │   ├── util.supportsHover.ts
│   │   │   ├── util.upload.ts
│   │   │   └── uuid.ts
│   │   ├── personalUtils/
│   │   │   ├── logger/
│   │   │   │   ├── README.md
│   │   │   │   ├── logger.config.ts
│   │   │   │   └── logger.ts
│   │   │   ├── personal.util.blocks.ts
│   │   │   ├── personal.util.chatActions.ts
│   │   │   ├── personal.util.contactActions.ts
│   │   │   ├── personal.util.contacts.ts
│   │   │   ├── personal.util.device.ts
│   │   │   ├── personal.util.profile.ts
│   │   │   ├── util.avatarCache.ts
│   │   │   ├── util.avatarCommon.ts
│   │   │   ├── util.chatErrors.ts
│   │   │   ├── util.chatMedia.ts
│   │   │   ├── util.chatPreview.ts
│   │   │   ├── util.contactMessages.ts
│   │   │   ├── util.messageTick.ts
│   │   │   └── util.profileAvatar.ts
│   │   └── publicUtils/
│   │       └── public.util.profile.ts
│   └── global.css
├── .env
├── .gitignore
├── .gitnexusignore
├── AGENTS.md
├── CLAUDE.md
├── README.md
├── app.json
├── babel.config.js
├── buf.gen.yaml
├── eas.json
├── eslint.config.js
├── expo-env.d.ts
├── google-services.json
├── index.ts
├── jest.config.js
├── legend-babel.d.ts
├── package-lock.json
├── package.json
├── tsconfig.json
├── unistyles.ts
└── wrangler.toml
```
