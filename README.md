# Bitrise CodePush

[![Step changelog](https://shields.io/github/v/release/bitrise-steplib/steps-bitrise-codepush?include_prereleases&label=changelog&color=blueviolet)](https://github.com/bitrise-steplib/steps-bitrise-codepush/releases)

Bundle React Native or Expo JavaScript and push it as an OTA update to Bitrise CodePush.

<details>
<summary>Description</summary>

Bundles the JavaScript code (and assets) of a React Native or Expo project and publishes it as
an over-the-air (OTA) update to a [Bitrise CodePush](https://bitrise.io) deployment, so devices
running the [CodePush SDK](https://github.com/bitrise-io/bitrise-plugins-codepush-cli) can pick
it up without going through an app store release.

This is the officially supported way to publish CodePush updates from a Bitrise build. It
replaces installing the `bitrise-plugins-codepush-cli` CLI plugin at runtime or shelling out to
the `release-management-recipes` reference script: this Step ports the same publishing logic
natively, with typed inputs, secret handling, and artifact export.

### Configuring the Step

1. Add the Step to a Workflow after your JS dependencies are installed (or leave
   **Skip dependency install** unchecked and let the Step run the install for you).
2. Set **Target platform** to `ios` or `android`. Each run only bundles and publishes for one
   platform; to update both iOS and Android, add the Step twice, once per platform.
3. Set **CodePush app ID** and **Deployment** (name, e.g. `Staging`, or UUID) for the app you're
   publishing to.
4. Set **Target app version** to the native app version this update targets (e.g. `1.2.0`).
5. Set **Bitrise API token** as a [Secret](https://devcenter.bitrise.io/en/builds/env-vars-and-secrets/adding-and-managing-secrets.html).
6. Optionally set **Rollout percentage**, **Mandatory update**, and **Disable after upload** to
   control how the update is rolled out.

The Step auto-detects your project type (React Native or Expo), entry file, and Hermes bytecode
configuration; overrides are available under **Bundling options** if auto-detection doesn't fit
your project layout.

The built update package is also exported under `$BITRISE_DEPLOY_DIR`, so a subsequent
**Deploy to Bitrise.io** Step picks it up automatically and it shows up on the build's Artifacts
tab, with no extra wiring needed.

### What's not (yet) supported

This first version covers publishing a new update (Phase 1 of the
[project brief](https://bitrise.atlassian.net/wiki/spaces/RD/pages/5151653927)). Code signing,
deployment management (create/rename/list), rollback, and promote are not part of this Step yet.

### Useful links

- [About CodePush](https://github.com/bitrise-io/bitrise-plugins-codepush-cli)
- [Using the Hermes engine, Expo Documentation](https://docs.expo.dev/guides/using-hermes/)
</details>

## 🧩 Get started

Add this step directly to your workflow in the [Bitrise Workflow Editor](https://docs.bitrise.io/en/bitrise-ci/workflows-and-pipelines/steps/adding-steps-to-a-workflow.html).

You can also run this step directly with [Bitrise CLI](https://github.com/bitrise-io/bitrise).

## ⚙️ Configuration

<details>
<summary>Inputs</summary>

| Key | Description | Flags | Default |
| --- | --- | --- | --- |
| `platform` | The platform the JavaScript bundle and update target.  Each run of the Step bundles and publishes for a single platform (the bundle filename and entry file differ between iOS and Android, and a CodePush update carries one bundle). To publish updates for both iOS and Android, add the Step twice in your Workflow, once per platform. | required | `ios` |
| `app_id` | The UUID of the Bitrise CodePush app to publish this update to.  Find this on your app's CodePush page, or via `CODEPUSH_APP_ID` if you already export it as a Workflow env var. | required | `$CODEPUSH_APP_ID` |
| `deployment` | The name (e.g. `Staging`, `Production`) or UUID of the deployment to publish the update to. | required |  |
| `app_version` | The target native binary version for this update, e.g. `1.2.0`.  Devices only receive this update if their installed native app version matches. This is usually your app's marketing/short version string, not the build number. | required |  |
| `api_token` | A Bitrise API access token with access to the app the update is being published to.  Generate one under **Account Settings > Security** on [bitrise.io](https://app.bitrise.io/me/account/security). | required, sensitive |  |
| `rollout_percentage` | Percentage of devices, between 0 and 100, that receive this update immediately.  Leave at 100 to roll the update out to all devices right away, or set a lower value to ramp up a staged rollout over time (adjustable later from the CodePush dashboard). |  | `100` |
| `mandatory` | When enabled, devices are forced to install this update (a "mandatory" or "forced" update) instead of being able to defer it. |  | `false` |
| `disabled_after_upload` | When enabled, the update is uploaded but not served to any device until manually enabled later (e.g. from the CodePush dashboard). Useful for a "dark push" you plan to enable after separate verification. |  | `false` |
| `description` | Optional release notes / description for this update. |  |  |
| `project_dir` | The root directory of the React Native or Expo project to bundle, containing its `package.json`. |  | `$BITRISE_SOURCE_DIR` |
| `entry_file` | Path to the JavaScript entry file, relative to **Project directory**.  Leave empty to auto-detect (`index.<platform>.js`, then `index.js`, then the `main` field in `package.json`). |  |  |
| `hermes` | Controls whether the JavaScript bundle is compiled to Hermes bytecode after bundling.  - `auto`: detect from the project's `android/app/build.gradle` / `ios/Podfile`, falling   back to "enabled" for React Native >= 0.70 (where Hermes is the default engine) if no   explicit setting is found. - `on`: always compile to Hermes bytecode. - `off`: never compile to Hermes bytecode.  The compiled bundle must match the Hermes/JSC engine the native app was built with, or the update will fail to load on-device. |  | `auto` |
| `bundle_name` | Overrides the output filename of the generated JS bundle.  Leave empty to use the platform default (`main.jsbundle` for iOS, `index.android.bundle` for Android), or the filename auto-detected from your native project files (Expo only). |  |  |
| `skip_dependency_install` | When enabled, skips running `npm install` / `yarn install` / `pnpm install` / `bun install` before bundling. Enable this if a previous Step in the Workflow already installed the project's JS dependencies. |  | `false` |
| `server_url` | Base URL of the Bitrise API server. Override only for testing against a non-production environment. |  | `https://api.bitrise.io` |
| `verbose_log` | Enable this to print additional information useful for debugging. |  | `false` |
</details>

<details>
<summary>Outputs</summary>

| Environment Variable | Description |
| --- | --- |
| `BITRISE_CODEPUSH_UPDATE_ID` | The package/update ID assigned to the pushed release. |
| `BITRISE_CODEPUSH_APP_VERSION` | The native app version the pushed update targets. |
| `BITRISE_CODEPUSH_PACKAGE_PATH` | Local path to the built and uploaded update package (a `.zip`). The Step copies it under `$BITRISE_DEPLOY_DIR`, so a subsequent **Deploy to Bitrise.io** Step picks it up automatically and it appears on the build's Artifacts tab. |
</details>

## 🙋 Contributing

We welcome [pull requests](https://github.com/bitrise-steplib/steps-bitrise-codepush/pulls) and [issues](https://github.com/bitrise-steplib/steps-bitrise-codepush/issues) against this repository.

For pull requests, work on your changes in a forked repository and use the Bitrise CLI to [run step tests locally](https://docs.bitrise.io/en/bitrise-ci/bitrise-cli/running-your-first-local-build-with-the-cli.html).

Learn more about developing steps:

- [Create your own step](https://docs.bitrise.io/en/bitrise-ci/workflows-and-pipelines/developing-your-own-bitrise-step/developing-a-new-step.html)
