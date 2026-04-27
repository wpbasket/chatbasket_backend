# Backend Folder Structure

```
chatbasket_backend/
├── .agents/
│   ├── rules/
│   │   └── intelligence.md
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
│   │   │   │   └── react-compiler.md
│   │   │   └── SKILL.md
│   │   └── use-dom/
│   │       └── SKILL.md
│   └── workflows/
│       ├── folder-tree-sync.md
│       ├── gitnexus-backup-cb-docs-references.md
│       ├── gitnexus-cb.md
│       ├── gitnexus-chatbasket-backend.md
│       ├── gitnexus-chatbasket.md
│       ├── gitnexus-helper-cb-backend.md
│       ├── gitnexus-wiki-backup-cb-docs-references.md
│       ├── gitnexus-wiki-chatbasket-backend.md
│       ├── gitnexus-wiki-chatbasket.md
│       └── gitnexus-wiki-helper-cb-backend.md
├── .claude/
│   ├── agents/
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
│   │   │   │   └── react-compiler.md
│   │   │   └── SKILL.md
│   │   └── use-dom/
│   │       └── SKILL.md
│   ├── settings.json
│   └── settings.local.json
├── .github/
│   └── workflows/
│       ├── deploy_relay.yml
│       └── deploy_web.yml
├── .gitnexus/
│   ├── lbug
│   └── meta.json
├── .junie/
│   ├── plans/
│   └── skills/
│       ├── building-native-ui/
│       │   ├── references/
│       │   │   ├── animations.md
│       │   │   ├── controls.md
│       │   │   ├── form-sheet.md
│       │   │   ├── gradients.md
│       │   │   ├── icons.md
│       │   │   ├── media.md
│       │   │   ├── route-structure.md
│       │   │   ├── search.md
│       │   │   ├── storage.md
│       │   │   ├── tabs.md
│       │   │   ├── toolbar-and-headers.md
│       │   │   ├── visual-effects.md
│       │   │   ├── webgpu-three.md
│       │   │   └── zoom-transitions.md
│       │   └── SKILL.md
│       ├── expo-api-routes/
│       │   └── SKILL.md
│       ├── expo-cicd-workflows/
│       │   ├── scripts/
│       │   │   ├── fetch.js
│       │   │   ├── package.json
│       │   │   └── validate.js
│       │   └── SKILL.md
│       ├── expo-deployment/
│       │   ├── references/
│       │   │   ├── app-store-metadata.md
│       │   │   ├── ios-app-store.md
│       │   │   ├── play-store.md
│       │   │   ├── testflight.md
│       │   │   └── workflows.md
│       │   └── SKILL.md
│       ├── expo-dev-client/
│       │   └── SKILL.md
│       ├── expo-module/
│       │   ├── references/
│       │   │   ├── config-plugin.md
│       │   │   ├── lifecycle.md
│       │   │   ├── module-config.md
│       │   │   ├── native-module.md
│       │   │   └── native-view.md
│       │   └── SKILL.md
│       ├── expo-tailwind-setup/
│       │   └── SKILL.md
│       ├── expo-ui-jetpack-compose/
│       │   └── SKILL.md
│       ├── expo-ui-swiftui/
│       │   └── SKILL.md
│       ├── frontend-design/
│       │   ├── LICENSE.txt
│       │   └── SKILL.md
│       ├── native-data-fetching/
│       │   ├── references/
│       │   │   └── expo-router-loaders.md
│       │   └── SKILL.md
│       ├── ui-ux-pro-max/
│       │   ├── SKILL.md
│       │   ├── data
│       │   └── scripts
│       ├── upgrading-expo/
│       │   ├── references/
│       │   │   ├── expo-av-to-audio.md
│       │   │   ├── expo-av-to-video.md
│       │   │   ├── native-tabs.md
│       │   │   ├── new-architecture.md
│       │   │   ├── react-19.md
│       │   │   └── react-compiler.md
│       │   └── SKILL.md
│       └── use-dom/
│           └── SKILL.md
├── chatbasket-api/
│   ├── app/
│   │   └── main.go
│   ├── db/
│   │   ├── common/
│   │   │   ├── migrations/
│   │   │   │   ├── 001_auth_init.down.sql
│   │   │   │   └── 001_auth_init.up.sql
│   │   │   └── queries/
│   │   │       └── auth.sql
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
│   │   │   │   └── 007_block_sync_actions_cleanup.up.sql
│   │   │   └── queries/
│   │   │       ├── personal_chat.sql
│   │   │       ├── personal_contacts.sql
│   │   │       └── personal_profile.sql
│   │   └── public/
│   │       ├── migrations/
│   │       └── queries/
│   │           └── placeholder.sql
│   ├── internal/
│   │   ├── modules/
│   │   │   ├── core/
│   │   │   │   └── core_auth/
│   │   │   │       ├── internal/
│   │   │   │       │   └── core_auth_store/
│   │   │   │       │       ├── auth.sql.go
│   │   │   │       │       ├── db.go
│   │   │   │       │       ├── models.go
│   │   │   │       │       └── querier.go
│   │   │   │       ├── core_auth_api_common_http_handler.go
│   │   │   │       ├── core_auth_api_http_handler.go
│   │   │   │       ├── core_auth_api_routes.go
│   │   │   │       ├── core_auth_errors.go
│   │   │   │       ├── core_auth_kit_otp.go
│   │   │   │       ├── core_auth_kit_password.go
│   │   │   │       ├── core_auth_mdl.go
│   │   │   │       ├── core_auth_mdl_common.go
│   │   │   │       ├── core_auth_mdl_helpers.go
│   │   │   │       ├── core_auth_svc.go
│   │   │   │       ├── core_auth_svc_common.go
│   │   │   │       ├── core_auth_svc_flows.go
│   │   │   │       ├── core_auth_svc_helpers.go
│   │   │   │       └── core_auth_svc_middleware.go
│   │   │   ├── personal/
│   │   │   │   ├── personal_chat/
│   │   │   │   │   ├── internal/
│   │   │   │   │   │   └── personal_chat_store/
│   │   │   │   │   │       ├── db.go
│   │   │   │   │   │       ├── models.go
│   │   │   │   │   │       ├── personal_chat.sql.go
│   │   │   │   │   │       └── querier.go
│   │   │   │   │   ├── personal_chat_api_http_handler.go
│   │   │   │   │   ├── personal_chat_api_routes.go
│   │   │   │   │   ├── personal_chat_api_ws_handler.go
│   │   │   │   │   ├── personal_chat_api_ws_router.go
│   │   │   │   │   ├── personal_chat_errors.go
│   │   │   │   │   ├── personal_chat_mdl.go
│   │   │   │   │   ├── personal_chat_svc.go
│   │   │   │   │   ├── personal_chat_svc_cleanup.go
│   │   │   │   │   └── personal_chat_svc_file.go
│   │   │   │   ├── personal_contact/
│   │   │   │   │   ├── internal/
│   │   │   │   │   │   └── personal_contact_store/
│   │   │   │   │   │       ├── db.go
│   │   │   │   │   │       ├── models.go
│   │   │   │   │   │       ├── personal_contacts.sql.go
│   │   │   │   │   │       └── querier.go
│   │   │   │   │   ├── personal_contact_api_http_handler.go
│   │   │   │   │   ├── personal_contact_api_routes.go
│   │   │   │   │   ├── personal_contact_errors.go
│   │   │   │   │   ├── personal_contact_mdl.go
│   │   │   │   │   ├── personal_contact_svc.go
│   │   │   │   │   └── personal_contact_svc_helpers.go
│   │   │   │   ├── personal_profile/
│   │   │   │   │   ├── internal/
│   │   │   │   │   │   └── personal_profile_store/
│   │   │   │   │   │       ├── db.go
│   │   │   │   │   │       ├── models.go
│   │   │   │   │   │       ├── personal_profile.sql.go
│   │   │   │   │   │       └── querier.go
│   │   │   │   │   ├── personal_profile_api_http_handler.go
│   │   │   │   │   ├── personal_profile_api_routes.go
│   │   │   │   │   ├── personal_profile_errors.go
│   │   │   │   │   ├── personal_profile_mdl.go
│   │   │   │   │   ├── personal_profile_svc.go
│   │   │   │   │   └── personal_profile_svc_helpers.go
│   │   │   │   └── personal_setting/
│   │   │   │       ├── personal_setting_api_http_handler.go
│   │   │   │       ├── personal_setting_api_routes.go
│   │   │   │       ├── personal_setting_errors.go
│   │   │   │       ├── personal_setting_mdl.go
│   │   │   │       └── personal_setting_svc.go
│   │   │   └── public/
│   │   └── platform/
│   │       ├── clients/
│   │       │   ├── appwrite.go
│   │       │   ├── cosmos.go
│   │       │   ├── email.go
│   │       │   ├── firebase.go
│   │       │   ├── postgres.go
│   │       │   └── secrets.go
│   │       ├── config/
│   │       │   └── config.go
│   │       ├── kit/
│   │       │   ├── crypto.go
│   │       │   ├── errors.go
│   │       │   ├── models.go
│   │       │   └── utils.go
│   │       ├── logger/
│   │       │   └── logger.go
│   │       ├── middleware/
│   │       │   ├── auth_session_middleware.go
│   │       │   └── middleware.go
│   │       ├── router/
│   │       │   └── routes.go
│   │       ├── services/
│   │       │   ├── services.go
│   │       │   └── storage_svc.go
│   │       └── websocket/
│   │           └── websocket.go
│   ├── .env
│   ├── .gitignore
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   └── sqlc.yaml
├── chatbasket-api-legacy/
│   ├── app/
│   │   ├── README.md
│   │   └── main.go
│   ├── appwriteinternal/
│   │   ├── APPWRITE_FILE_SYSTEM_REFERENCE.md
│   │   ├── README.md
│   │   ├── service.go
│   │   ├── service_session.go
│   │   └── service_storage.go
│   ├── common/
│   │   ├── commonhandler/
│   │   │   ├── auth_handler.go
│   │   │   └── setting_handler.go
│   │   ├── commonmodel/
│   │   │   └── commonModel.go
│   │   └── commonservice/
│   │       ├── auth_service.go
│   │       ├── service.go
│   │       └── setting_service.go
│   ├── db/
│   │   ├── auth/
│   │   │   ├── migrations/
│   │   │   │   ├── 001_auth_init.down.sql
│   │   │   │   └── 001_auth_init.up.sql
│   │   │   ├── queries/
│   │   │   │   └── auth.sql
│   │   │   └── sqlc.yaml
│   │   ├── personal/
│   │   │   ├── migrations/
│   │   │   │   ├── 001_personal_init.down.sql
│   │   │   │   ├── 001_personal_init.up.sql
│   │   │   │   ├── 002_personal_user_contacts.down.sql
│   │   │   │   ├── 002_personal_user_contacts.up.sql
│   │   │   │   ├── 003_personal_user_restrictions.down.sql
│   │   │   │   ├── 003_personal_user_restrictions.up.sql
│   │   │   │   ├── 004_personal_user_blocks.down.sql
│   │   │   │   ├── 004_personal_user_blocks.up.sql
│   │   │   │   ├── 005_personal_global_restrictions.down.sql
│   │   │   │   ├── 005_personal_global_restrictions.up.sql
│   │   │   │   ├── 006_personal_contact_requests.down.sql
│   │   │   │   ├── 006_personal_contact_requests.up.sql
│   │   │   │   ├── 007_personal_tokens.down.sql
│   │   │   │   ├── 007_personal_tokens.up.sql
│   │   │   │   ├── 008_personal_chat_system.down.sql
│   │   │   │   └── 008_personal_chat_system.up.sql
│   │   │   ├── queries/
│   │   │   │   ├── personal_chat.sql
│   │   │   │   ├── personal_contacts.sql
│   │   │   │   └── personal_user.sql
│   │   │   └── sqlc.yaml
│   │   ├── public/
│   │   │   ├── migrations/
│   │   │   │   └── 001_public_init.up.sql
│   │   │   ├── queries/
│   │   │   │   └── .gitkeep
│   │   │   └── sqlc.yaml
│   │   ├── README.md
│   │   ├── config.go
│   │   ├── cosmos_client.go
│   │   ├── cosmos_config.go
│   │   └── pool.go
│   ├── handler/
│   │   ├── README.md
│   │   └── user_handler.go
│   ├── internal/
│   │   └── db/
│   │       ├── auth/
│   │       │   ├── auth.sql.go
│   │       │   ├── db.go
│   │       │   └── models.go
│   │       ├── personal/
│   │       │   ├── db.go
│   │       │   ├── models.go
│   │       │   ├── personal_chat.sql.go
│   │       │   ├── personal_contacts.sql.go
│   │       │   └── personal_user.sql.go
│   │       └── public/
│   ├── middleware/
│   │   ├── README.md
│   │   └── session.go
│   ├── model/
│   │   ├── base.go
│   │   ├── block.go
│   │   ├── comment.go
│   │   ├── error.go
│   │   ├── follow.go
│   │   ├── follow_request.go
│   │   ├── like.go
│   │   ├── post.go
│   │   ├── profile.go
│   │   ├── resend_otp.go
│   │   ├── settings.go
│   │   ├── token.go
│   │   └── user.go
│   ├── personal/
│   │   ├── personalhandler/
│   │   │   ├── chat_handler.go
│   │   │   ├── contact_handler.go
│   │   │   ├── profile_handler.go
│   │   │   ├── setting_handler.go
│   │   │   └── ws_handler.go
│   │   ├── personalmodel/
│   │   │   ├── chat_models.go
│   │   │   ├── contactPersModel.go
│   │   │   └── profilePersModel.go
│   │   ├── personalservice/
│   │   │   ├── chat_file_service.go
│   │   │   ├── chat_service.go
│   │   │   ├── contact_service.go
│   │   │   ├── profile_service.go
│   │   │   ├── service.go
│   │   │   ├── setting_service.go
│   │   │   ├── ws_hub.go
│   │   │   └── ws_router.go
│   │   └── personalutils/
│   │       ├── message_cleanup.go
│   │       └── usernamePersUtils.go
│   ├── public/
│   │   ├── publichandler/
│   │   │   └── profile_handler.go
│   │   ├── publicmodel/
│   │   │   └── README.md
│   │   ├── publicservice/
│   │   │   ├── profile_service.go
│   │   │   └── service.go
│   │   └── publicutils/
│   │       └── README.md
│   ├── routes/
│   │   ├── common_routes.go
│   │   ├── config.go
│   │   ├── personal_routes.go
│   │   ├── public_routes.go
│   │   └── routes.go
│   ├── services/
│   │   ├── README.md
│   │   ├── auth_base_service.go
│   │   ├── base_service.go
│   │   ├── fcm_service.go
│   │   ├── upload.go
│   │   └── user_service.go
│   ├── utils/
│   │   ├── README.md
│   │   ├── auth_flow_utils.go
│   │   ├── baseUtils.go
│   │   ├── emailUtils.go
│   │   ├── errorUtils.go
│   │   ├── firebase_config.go
│   │   ├── hashingTextUtils.go
│   │   ├── otpUtils.go
│   │   ├── passwordUtils.go
│   │   └── toInputFileUtils.go
│   ├── .dockerignore
│   ├── .env
│   ├── .gitignore
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
├── deployment/
│   ├── docker-compose.yml
│   └── nginx.conf
├── docs/
│   ├── business-rules/
│   │   ├── common/
│   │   ├── personal/
│   │   │   ├── personal.chat-system.md
│   │   │   └── personal.profile&contact-system.md
│   │   └── public/
│   ├── BACKEND_CONSISTENCY.md
│   └── CHANGE_POLICY.md
├── heroku-mail-relay/
│   ├── app/
│   │   └── main.go
│   ├── Dockerfile
│   └── go.mod
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
│   ├── rules/
│   │   └── intelligence.md
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
│   │   │   │   └── react-compiler.md
│   │   │   └── SKILL.md
│   │   └── use-dom/
│   │       └── SKILL.md
│   └── workflows/
│       ├── folder-tree-sync.md
│       ├── gitnexus-backup-cb-docs-references.md
│       ├── gitnexus-cb.md
│       ├── gitnexus-chatbasket-backend.md
│       ├── gitnexus-chatbasket.md
│       ├── gitnexus-helper-cb-backend.md
│       ├── gitnexus-wiki-backup-cb-docs-references.md
│       ├── gitnexus-wiki-chatbasket-backend.md
│       ├── gitnexus-wiki-chatbasket.md
│       └── gitnexus-wiki-helper-cb-backend.md
├── .claude/
│   ├── agents/
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
│   │   │   │   └── react-compiler.md
│   │   │   └── SKILL.md
│   │   └── use-dom/
│   │       └── SKILL.md
│   ├── settings.json
│   └── settings.local.json
├── .gitnexus/
│   ├── lbug
│   └── meta.json
├── .junie/
│   ├── plans/
│   └── skills/
│       ├── building-native-ui/
│       │   ├── references/
│       │   │   ├── animations.md
│       │   │   ├── controls.md
│       │   │   ├── form-sheet.md
│       │   │   ├── gradients.md
│       │   │   ├── icons.md
│       │   │   ├── media.md
│       │   │   ├── route-structure.md
│       │   │   ├── search.md
│       │   │   ├── storage.md
│       │   │   ├── tabs.md
│       │   │   ├── toolbar-and-headers.md
│       │   │   ├── visual-effects.md
│       │   │   ├── webgpu-three.md
│       │   │   └── zoom-transitions.md
│       │   └── SKILL.md
│       ├── expo-api-routes/
│       │   └── SKILL.md
│       ├── expo-cicd-workflows/
│       │   ├── scripts/
│       │   │   ├── fetch.js
│       │   │   ├── package.json
│       │   │   └── validate.js
│       │   └── SKILL.md
│       ├── expo-deployment/
│       │   ├── references/
│       │   │   ├── app-store-metadata.md
│       │   │   ├── ios-app-store.md
│       │   │   ├── play-store.md
│       │   │   ├── testflight.md
│       │   │   └── workflows.md
│       │   └── SKILL.md
│       ├── expo-dev-client/
│       │   └── SKILL.md
│       ├── expo-module/
│       │   ├── references/
│       │   │   ├── config-plugin.md
│       │   │   ├── lifecycle.md
│       │   │   ├── module-config.md
│       │   │   ├── native-module.md
│       │   │   └── native-view.md
│       │   └── SKILL.md
│       ├── expo-tailwind-setup/
│       │   └── SKILL.md
│       ├── expo-ui-jetpack-compose/
│       │   └── SKILL.md
│       ├── expo-ui-swiftui/
│       │   └── SKILL.md
│       ├── frontend-design/
│       │   ├── LICENSE.txt
│       │   └── SKILL.md
│       ├── native-data-fetching/
│       │   ├── references/
│       │   │   └── expo-router-loaders.md
│       │   └── SKILL.md
│       ├── ui-ux-pro-max/
│       │   ├── SKILL.md
│       │   ├── data
│       │   └── scripts
│       ├── upgrading-expo/
│       │   ├── references/
│       │   │   ├── expo-av-to-audio.md
│       │   │   ├── expo-av-to-video.md
│       │   │   ├── native-tabs.md
│       │   │   ├── new-architecture.md
│       │   │   ├── react-19.md
│       │   │   └── react-compiler.md
│       │   └── SKILL.md
│       └── use-dom/
│           └── SKILL.md
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
│       ├── favicon.png
│       ├── icon.png
│       ├── partial-react-logo.png
│       ├── react-logo.png
│       ├── react-logo@2x.png
│       ├── react-logo@3x.png
│       └── splash-icon.png
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
│   └── fix-cloudflare-pages.ts
├── src/
│   ├── __tests__/
│   │   ├── lib/
│   │   │   └── storage/
│   │   │       └── personalStorage/
│   │   │           └── chat/
│   │   └── state/
│   │       ├── chat/
│   │       │   ├── ack_race_condition.test.ts
│   │       │   ├── avatar_caching.test.ts
│   │       │   └── is_contactable.test.ts
│   │       └── personalState/
│   │           ├── chat/
│   │           └── profile_avatar.test.ts
│   ├── app/
│   │   ├── (auth)/
│   │   │   ├── auth-verify.tsx
│   │   │   ├── auth.tsx
│   │   │   └── index.tsx
│   │   ├── personal/
│   │   │   ├── contacts/
│   │   │   │   ├── components/
│   │   │   │   │   ├── ContactRow.tsx
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
│   │   └── index.tsx
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
│   │   ├── personalComponents/
│   │   │   └── chat/
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
│   │   │   │   └── external-link.tsx
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
│   ├── hooks/
│   │   ├── commonHooks/
│   │   │   ├── hooks.notificationPermission.ts
│   │   │   ├── hooks.notificationPermission.web.ts
│   │   │   └── hooks.pressableAnimation.ts
│   │   ├── personalHooks/
│   │   │   └── hooks.stableIme.ts
│   │   ├── publicHooks/
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
│   │   │   │   ├── __tests__/
│   │   │   │   ├── chat.transport.ts
│   │   │   │   ├── connection.watcher.ts
│   │   │   │   ├── outbox.queue.ts
│   │   │   │   ├── personal.api.chat.ts
│   │   │   │   └── ws.client.ts
│   │   │   ├── constant/
│   │   │   │   └── constant.chat.ts
│   │   │   ├── contactApi/
│   │   │   │   └── personal.api.contact.ts
│   │   │   ├── fileSystem/
│   │   │   │   ├── file.copy.ts
│   │   │   │   └── file.download.ts
│   │   │   ├── models/
│   │   │   │   ├── personal.model.chat.ts
│   │   │   │   ├── personal.model.contact.ts
│   │   │   │   ├── personal.model.notification.ts
│   │   │   │   ├── personal.model.profile.ts
│   │   │   │   └── personal.model.setting.ts
│   │   │   ├── profileApi/
│   │   │   │   └── personal.api.profile.ts
│   │   │   ├── settingApi/
│   │   │   │   └── personal.api.setting.ts
│   │   │   └── index.ts
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
│   │   │   │   └── storage.preferences.ts
│   │   │   ├── personalStorage/
│   │   │   │   ├── chat/
│   │   │   │   │   ├── __tests__/
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
│   │   │   ├── publicStorage/
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
│   │   │   └── state.auth.ts
│   │   ├── modals/
│   │   │   └── state.modals.ts
│   │   ├── personalState/
│   │   │   ├── chat/
│   │   │   │   ├── personal.state.chat.ts
│   │   │   │   ├── personal.state.sync.ts
│   │   │   │   └── ws.event.bridge.ts
│   │   │   ├── contacts/
│   │   │   │   └── personal.state.contacts.ts
│   │   │   ├── home/
│   │   │   │   └── personal.state.home.ts
│   │   │   ├── profile/
│   │   │   │   ├── personal.state.profile.createProfile.ts
│   │   │   │   └── personal.state.profile.updateProfile.ts
│   │   │   └── user/
│   │   │       └── personal.state.user.ts
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
│   │   ├── tools/
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
│   │   │   └── util.upload.ts
│   │   ├── personalUtils/
│   │   │   ├── logger/
│   │   │   │   ├── README.md
│   │   │   │   ├── logger.config.ts
│   │   │   │   └── logger.ts
│   │   │   ├── personal.util.contacts.ts
│   │   │   ├── personal.util.device.ts
│   │   │   ├── personal.util.profile.ts
│   │   │   ├── util.avatarCache.ts
│   │   │   ├── util.avatarCommon.ts
│   │   │   ├── util.chatErrors.ts
│   │   │   ├── util.chatMedia.ts
│   │   │   ├── util.chatPreview.ts
│   │   │   ├── util.contactMessages.ts
│   │   │   └── util.profileAvatar.ts
│   │   └── publicUtils/
│   │       └── public.util.profile.ts
│   └── global.css
├── .env
├── .gitignore
├── AGENTS.md
├── CLAUDE.md
├── README.md
├── app.json
├── babel.config.js
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
