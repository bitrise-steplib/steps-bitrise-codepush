# Bitrise CodePush Publish

[![Step changelog](https://shields.io/github/v/release/bitrise-steplib/steps-bitrise-codepush?include_prereleases&label=changelog&color=blueviolet)](https://github.com/bitrise-steplib/steps-bitrise-codepush/releases)

Bundle React Native or Expo JavaScript and push it as an OTA update to Bitrise CodePush.

<details>
<summary>Description</summary>

Bundles the JavaScript code (and assets) of a React Native project and publishes it as an
over-the-air (OTA) update to a Bitrise CodePush deployment.

### Configuring the Step

1. Add the Step to a Workflow after your JS dependencies are installed (or leave
   **Skip dependency install** unchecked and let the Step run the install for you).
2. Set **Target platform** to `ios` or `android`. Each run only bundles for one platform.
3. Set **CodePush app ID** and **Deployment** (name, e.g. `Staging`, or UUID) for the app you're
   working with.
4. Set **Bitrise API token** as a [Secret](https://devcenter.bitrise.io/en/builds/env-vars-and-secrets/adding-and-managing-secrets.html).

The Step auto-detects your project's entry file and Hermes bytecode configuration; overrides
are available under **Bundling options** if auto-detection doesn't fit your project layout.
Expo projects are supported too — the Step detects Expo vs. bare React Native automatically
from your project's `package.json`.

The built package is exported under `$BITRISE_DEPLOY_DIR`, so a subsequent
**Deploy to Bitrise.io** Step picks it up automatically and it shows up on the build's Artifacts
tab, with no extra wiring needed.

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
| `platform` | The platform the JavaScript bundle and update target.  Each run of the Step bundles for a single platform (the bundle filename and entry file differ between iOS and Android). | required | `ios` |
| `app_id` | The UUID of the Bitrise CodePush app this update targets.  Find this on your app's CodePush page, or via `CODEPUSH_APP_ID` if you already export it as a Workflow env var. | required | `$CODEPUSH_APP_ID` |
| `deployment` | The name (e.g. `Staging`, `Production`) or UUID of the deployment to work with. | required |  |
| `api_token` | A Bitrise API access token with access to the app above.  Prefer a **Workspace API token**: unlike a personal access token, it isn't tied to any one team member's account, so it keeps working if that person leaves or their access changes. A **Personal Access Token** (generated under **Account Settings > Security** on [bitrise.io](https://app.bitrise.io/me/account/security)) also works, but ties this Step's ability to publish updates to that individual's account. | required, sensitive |  |
| `project_dir` | The root directory of the React Native or Expo project to bundle, containing its `package.json`. |  | `$BITRISE_SOURCE_DIR` |
| `entry_file` | Path to the JavaScript entry file, relative to **Project directory**.  Leave empty to auto-detect (`index.<platform>.js`, then `index.js`, then the `main` field in `package.json`). |  |  |
| `hermes` | Controls whether the JavaScript bundle is compiled to Hermes bytecode after bundling.  - `auto`: detect from the project's `android/app/build.gradle` / `ios/Podfile`, falling   back to "enabled" for React Native >= 0.70 (where Hermes is the default engine) if no   explicit setting is found. - `on`: always compile to Hermes bytecode. - `off`: never compile to Hermes bytecode.  The compiled bundle must match the Hermes/JSC engine the native app was built with, or the update will fail to load on-device. |  | `auto` |
| `bundle_name` | Overrides the output filename of the generated JS bundle.  Leave empty to use the platform default (`main.jsbundle` for iOS, `index.android.bundle` for Android), or the filename auto-detected from your native project files (Expo only). |  |  |
| `skip_dependency_install` | When enabled, skips running `npm install` / `yarn install` / `pnpm install` / `bun install` before bundling. Enable this if a previous Step in the Workflow already installed the project's JS dependencies. |  | `false` |
| `verbose_log` | Enable this to print additional information useful for debugging. |  | `false` |
</details>

<details>
<summary>Outputs</summary>

| Environment Variable | Description |
| --- | --- |
| `BITRISE_CODEPUSH_PACKAGE_PATH` | Local path to the built update package (a `.zip`). The Step copies it under `$BITRISE_DEPLOY_DIR`, so a subsequent **Deploy to Bitrise.io** Step picks it up automatically and it appears on the build's Artifacts tab. |
</details>

## 🙋 Contributing

We welcome [pull requests](https://github.com/bitrise-steplib/steps-bitrise-codepush/pulls) and [issues](https://github.com/bitrise-steplib/steps-bitrise-codepush/issues) against this repository.

For pull requests, work on your changes in a forked repository and use the Bitrise CLI to [run step tests locally](https://docs.bitrise.io/en/bitrise-ci/bitrise-cli/running-your-first-local-build-with-the-cli.html).

Learn more about developing steps:

- [Create your own step](https://docs.bitrise.io/en/bitrise-ci/workflows-and-pipelines/developing-your-own-bitrise-step/developing-a-new-step.html)
