# Backend Folder Structure

```
chatbasket_backend/
├── .github/
│   └── workflows/
│       ├── deploy_relay.yml
│       └── deploy_web.yml
├── chatbasket-api/
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
│   │   │   │   ├── personal_tokens.sql
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
│   │       │   ├── personal_tokens.sql.go
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
│   │   │   ├── profile_handler.go
│   │   │   └── setting_handler.go
│   │   ├── publicmodel/
│   │   │   └── README.md
│   │   ├── publicservice/
│   │   │   ├── profile_service.go
│   │   │   ├── service.go
│   │   │   └── setting_service.go
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
│   │   │   └── personal.profile-system.md
│   │   └── public/
│   ├── BACKEND_CONSISTENCY.md
│   └── CHANGE_POLICY.md
├── heroku-mail-relay/
│   ├── app/
│   │   └── main.go
│   ├── Dockerfile
│   └── go.mod
└── README.md
```

# Frontend Folder Structure

```
chatbasket/
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
│   │   └── lib/
│   │       └── storage/
│   │           └── personalStorage/
│   │               └── chat/
│   │                   └── chat.storage.web.test.ts
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
│   │   │   └── common/
│   │   │       └── PrivacyAvatar.tsx
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
│   │   │   │   ├── chat.transport.ts
│   │   │   │   ├── connection.watcher.ts
│   │   │   │   ├── outbox.queue.ts
│   │   │   │   ├── personal.api.chat.ts
│   │   │   │   └── ws.client.ts
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
│   │   │   │   │   └── chat.storage.web.ts
│   │   │   │   ├── personal.storage.contacts.ts
│   │   │   │   ├── personal.storage.device.ts
│   │   │   │   └── personal.storage.user.ts
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
│   │   │   ├── util.chatErrors.ts
│   │   │   ├── util.chatMedia.ts
│   │   │   ├── util.chatPreview.ts
│   │   │   └── util.contactMessages.ts
│   │   └── publicUtils/
│   │       └── public.util.profile.ts
│   └── global.css
├── .env
├── .gitignore
├── .npmrc
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
