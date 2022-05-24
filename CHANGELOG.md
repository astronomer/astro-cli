# Changelog

All notable changes to this project will be documented in this file.
## [1.0.0] - 2022-06-23

### New Features
- The Astro CLI v1.0.0 works with [Astro](https://www.astronomer.io/product/), Astronomer's new cloud product. The Astro CLI deployment, workspace, and deploy commands will work with the Astro when a user logs into [astronomer.io](https://www.astronomer.io/). The function of these commands change slightly when logged into Astro. Details on how these commands change when working with Astro cna be found in the [Astro CLI Command Reference](https://docs.astronomer.io/astro/cli-reference)
- You can now specify a custom image name in your Astro project's Dockerfile as long as the image is based on an existing Astro Runtime image
- New `astro dev restart` command to test local changes. This command will restart your local Astro project
- After running `astro dev start`, the CLI know shows you the status of the Webserver container as it spins up on your local machine
- You can now run custom unit tests for all DAGs in your Astro project with `astro dev pytest`, a new Astro CLI command that uses pytest, a common testing framework for Python. As part of this change, new Astro projects created via astrocloud dev init now include a tests directory, which includes one example pytest built by Astronomer. In addition to running tests locally, you can also run pytest as part of the Astro deploy process. More information in [Release Notes](https://docs.astronomer.io/astro/cli-release-notes#new-command-to-run-dag-unit-tests-with-pytest)
- New `astro dev parse` command that allows you to run a basic test against your Astro project to ensure that your DAGs are able to to render in the Airflow UI. `astro deploy` now automatically applies tests from astrocloud dev parse to your Astro project before completing the deploy process. More information in [Release Notes](https://docs.astronomer.io/astro/cli-release-notes#new-command-to-parse-dags-for-errors)
- You can now use [Deployment API keys](https://docs.astronomer.io/astro/api-keys) to run `astro deploy` with Astro either from the CLI directly or via a CI/CD script. This update simplifies deploying code to Astro via CI/CD. More information in [Release Notes](https://docs.astronomer.io/astro/cli-release-notes#new-command-to-parse-dags-for-errors).

### Breaking changes
- `astro dev init`: the flag `-v` (used to provide a specific airflow version) has been moved to `-a` since `-v` is used to provide the runtime version for runtime users.
- `astro workspace create` now takes all workspace properties as flags instead of named arguments. Also, the `--desc` flag for description is renamed `--description`.
  ```bash
  # before
  astro workspace create label=my-workspace --desc="my description"
  # now
  astro workspace create --label=my-workspace --description="my description"
  ```
- `astro workspace update` now takes the workspace ID as an argument and the updated properties as flags (previously named arguments):
  ```bash
  # before
  astro workspace update ckw12x02830 label="new-label"
  # now
  astro workspace update ck21dx0834 --label="my-label" --description="updated description"
  ```
  Also, it is now possible to update the description of a workspace with the CLI.
- In every `astro workspace user` commands, you can now use the flag `--workspace-id` to provide the workspace ID in which you want to manage users. If this flag is not provided, these commands will read workspace ID from context.
- `astro workspace user add`: the email of the user you wish to add to the workspace is now passed as a flag instead of an argument:
  ```bash
  # before
  astro workspace user add email-to-add@astronomer.io --role WORKSPACE_VIEWER
  # now
  astro workspace user add --email email-to-add@astronomer.io --role WORKSPACE_VIEWER 
  ```
  Also, this new `--email` flag is required to run the command.
- In every `astro workspace sa` commands, you can now use the flag `--workspace-id` to provide the workspace ID in which you want to manage service accounts. If this flag is not provided, these commands will read workspace ID from context.
- `astro workspace sa get` becomes `astro workspace sa list` as we wish to harmonize the CLI commands by using the verb `list` to retrieve a list of objects
  ```bash
  # before
  astro workspace sa get
  # now
  astro workspace sa list
  ```
- In `astro workspace sa create`, the `--role` property accepted values has changed from `viewer|admin|editor` to `WORKSPACE_VIEWER|WORKSPACE_ADMIN|WORKSPACE_EDITOR`.
  ```bash
  # before
  astro workspace sa create --role viewer
  # now
  astro workspace sa create --role WORKSPACE_VIEWER
  ```
  This change has been made to harmonize the whole CLI project, because in other commands (`workspace user add` and `deployment user add`) we use the full permission name (DEPLOYMENT_VIEWER or WORKSPACE_VIEWER).
- `astro deployment create` now takes all deployment properties as flags (`--label=my-deployment-label` instead of argument)
  ```bash
  # before
  astro deployment create my-deployment --executor=local
  # now
  astro deployment create --label=my-deployment --executor=local
  ```
- `astro deployment update` now takes all deployment properties as flags instead of named arguments. Also, the `--description` flag has been added to allow users to update their deployment's description.
  ```bash
  # before
  astro deployment update ck129384ejf9 label=my-label --executor=celery
  # now
  astro deployment update ckw1289r0dake92 --label=new-label --description="My new description" --exectur=celery
  ```
- In `astro deployment sa create`, the `--role` property accepted values has changed from `viewer|admin|editor` to `DEPLOYMENT_VIEWER|DEPLOYMENT_ADMIN|DEPLOYMENT_EDITOR`.
  ```bash
  # before
  astro deployment sa create --role viewer
  # now
  astro deployment sa create --role DEPLOYMENT_VIEWER
  ```
  This change has been made to harmonize the whole CLI project, because in other commands (`workspace user add` and `deployment user add`) we use the full permission name (DEPLOYMENT_VIEWER or WORKSPACE_VIEWER).
- A few flags have been removed since they are not used anymore (`--system-sa` and `--user-id`)
- `astro deployment user` do not require a workspace set in context anymore (since each command will require a specific `--deployment-id` to manage users into).
- `astro deployment user list`: the deployment-id is now passed as a flag instead of an argument
  ```bash
  # before
  astro deployment user list <deployment-id>
  # now
  astro deployment user list --deployment-id=<deployment-id>
  ```
- `astro deployment user add`: the email of the user to add to the deployment is now passed as a flag `--email` (required flag) instead of an argument.
  ```bash
  # before
  astro deployment user add <email> --deployment-id=<deployment-id>
  # now
  astro deployment user add --email=<email> --deployment-id=<deployment-id>
  ```
- `astro deployment user delete` now becomes `astro deployment user remove`
  ```bash
  # before
  astro deployment user delete <email> --deployment-id=<deployment-id>
  # now
  astro deployment user remove <email> --deployment-id=<deployment-id>
  ```
- `astro logs <component>` commands have been deprecated for a long time now, and we wish to remove it.
  Instead, you can use the `astro deployment logs <component>` commands like in the following example:
  ```bash
  astro deployment logs webserver ckw129df023940 --since=1h
  ```
- Both `astro cluster list` and `astro cluster switch` commands has been deprecated and have been replaced by corresponding `astro context list` and `astro context switch` commands.
- `astro deploy` command will now accept deployment-id as argument instead of deployment release name, to make it consistent with other deployment commands
  ```bash
  # before
  astro deploy <release-name> -f
  # now
  astro deploy <deployment-id> -f
  ```
- Both `astro auth login` & `astro auth logout` has been renamed to `astro login` & `astro logout` respectively.
  ```bash
  # before
  astro auth login <domain>
  astro auth logout
  # after
  astro login <domain>
  astro logout
  ```

### Changes
- `astro context delete` command has been newly introduced in the CLI. The functionality of this command is to delete a particular locally stored context in `~/.astro/config.yaml` based on domain passed as argument to the command.
  ```bash
  # usage
  astro context delete <domain>
  ```
- This command has only one flag, which is `--force` which when passed assume \"yes\" as answer to all prompts which would pop up during command execution. As of now there is a prompt only when someone tries to delete a context which is current in use.


## [0.25.2] - 2021-06-23
- Add error message for airflow upgrade no-op #3055 (#417)

## [0.23.1] - 2020-11-25

- Fix/airflow settings update (#389)
- Fix/airflow settings update (#388)

## [0.23.0] - 2020-11-23

- Shorten list of recommended tags/images in CLI to 1 AC version #2166 (#386)

## [0.22.2] - 2020-11-17

- Use default as API Auth Backend (#387)

## [0.22.1] - 2020-11-16

- Fix message typo (#385)
- Hotfix warning when user push astronomerinc image repo (#384)
- Use basic-auth as API Auth Backend (#383)
- Run sync_perm / sync-perm command in Webserver (#381)

## [0.22.0] - 2020-11-05

- Add pre commit (#380)
- Switch docker hub references to quay.io (#379)
- change wording after astro deploy (#378)
- fix get selection airflow version (#377)
- s/airflowVersion/airflow-version (#376)
- Add --airflowVersion flag to $ astro deployment create and add astro deployment airflow upgrade --cancel
- Add deployment user list (#371)
- Add new codeowners (#374)
- Update codecov (#373)
- Update RegistryAuthFail message (#370)
- Update deployment user role (#367)
- Use Constant Instead of String (#369)
- Add feature Delete Deployment User Role (#368)
- Remove extra return (#366)
- Add RBAC Deployment User (#365)

## [0.21.0] - 2020-10-14

- Fix webserver command in docker-compose (#364)
- Make Example Plugin 2.0 compatible (#363)
- Make example-dag Airflow 2.0 compatible (#362)
- Add support for Airflow 2.0 (#361)
- Remove prisma1 (#360)

## [0.20.0] - 2020-09-16

- Update link to reference new repo name (#358)
- Update airflow dev init message (#356)
- Add compare versions to astro upgrade (#357)

## [0.16.4] - 2020-08-20

- Update airflow dev init message (#356)
- Add compare versions to astro upgrade (#357)
- Remove Houston version check on upgrade (#353)
- Fix incorrect hint flag
- hotfix: workspace user remove (#351)
- fix astro workspace user add (#350)
- Improve astro-cli UX when user trying to work locally without access houston (#347)

## [0.19.0] - 2020-08-20

- Remove Houston version check on upgrade (#353)
- Add pull request template (#354)
- Fix incorrect hint flag (#352)
- Workspace user remove (#351)
- fix astro workspace user add (#350)

## [0.18.0] - 2020-08-07

- Workspace user remove (#351)
- Fix astro workspace user add (#350)
- Deployment config schema change (#349)
- Remove update deployment sync flag (#348)

## [0.16.3] - 2020-08-07

- hotfix: workspace user remove (#351)
- fix astro workspace user add (#350)

## [0.16.2] - 2020-07-28

- Improve astro-cli UX when user trying to work locally without access houston (#347)

## [0.17.0] - 2020-07-24

- Improve astro-cli UX when user trying to work locally without access houston (#347)
- Increase code coverage to 50% (#342)
- Avoid validate compatibility during astro auth (#345)

## [0.16.1] - 2020-06-30

- Avoid validate compatibility during astro auth (#345)

## [0.16.0] - 2020-06-29

- Enforce Platform<->CLI 1:1 version mapping and display CLI upgrade/downgrade notice. (#339)
- Remove aliases no longer in schema (#336)
- Update README.md. Add brew to goreleaser. (#335)

## [0.15.0] - 2020-06-01

- Add missing sync=true during cloud role update (#334)
- Remove field no longer provided by Houston (#333)
- allow annotations to be applied to service account (#330)
- Registry Error on 'astro auth login' (#331)
- Add ability to manually set releaseName during create deployment (#327)
- Fix missing deployment service account (#329)
- Don't copy logs folder into built images (#326)
- Add missing check on error return from docker log in step (#325)

## [0.14.0] - 2020-05-13

- Add ability to manually set releaseName during create deployment (#327)
- Fix missing deployment service account (#329)
- Don't copy logs folder into built images (#326)
- Add missing check on error return from docker log in step (#325)

## [0.13.1] - 2020-05-13

- Registry Error on 'astro auth login' (#331)

## [0.13.0] - 2020-04-27

- Don't copy logs folder into built images (#326)
- Add missing check on error return from docker log in step (#325)

## [0.12.0] - 2020-03-09

- Add `conn_schema:` param to default airflow_settings.yaml file (#320)
- Remove brew (#322)
- Change link to get oauth token (#323)

## [0.11.0] - 2020-01-19

- Add column with tag name (#303)
- Add airflow upgradedb (#307)
- Query Houston for Airflow Image Tag (#308)
- Update example dag file (#315)
- Implement warning messages during airflow deploy (#314)

## [0.10.3] - 2019-10-28

- Negotiate docker api version (#304)
- Update Service Account Queries/Mutations (#300)
- Add initial integrations tests (#298)
- Add more examples for `astro deploy` (#302)
- Update example plugin to work with RBAC UI (#301)

## [0.10.2] - 2019-10-03

- bump gorelesaer fork version (#299)

## [0.10.1] - 2019-10-01

- Bump airflow version astronomer/issues#377 (#297)

## [0.10.0] - 2019-08-22

- Bump airflow version astronomer/issues#377 (#297)
- Change strategy of how goreleaser get last tag (#296)
- Add support section in README.md
- Fix corner case when same version without any changes in git ref log (#291)
- Update messages.go
- Fix error message when deployment name (#290)
- Revert "Show descriptive error message when no deployment is found (#287)" (#289)
- Adjust for new API format (#288)
- Show descriptive error message when no deployment is found (#287)
- Make workspace id required type (#286)
- Fix fetch all tags (#284)
- Add basic test to validate cobra (#283)
- Replace deployments to workspaceDeployments during fetch list of deployments (#282)
- `astro serviceaccount create` now fails with Role required (#280)
- Add ability to generate bash completion to astro cli (#277)
- Update version.go

## [0.9.0] - 2019-08-22

- Add helpful message when logging in with multiple workspaces
- Astro deploy not showing correct deployments (#276)
- Update workspace query (#274)
- Switch to safer "z" flag (#272)
- Use Z flag on volumes in docker compose for airflow (#271)
- Add CODEOWNERS (#266)
- Show error message instead of panic (#265)
- Fixing ignored filename (#262)
- Remove .astro from .gitignore template #260 (#261)
- Add executor flag to create deployment command (#252)
- Change error message to something more meaningful (#257)
- Update to use new AuthConfig/AuthProvider schema (#256)
- Update authConfig schema (#255)
- Minor edits to README
- Add astro dev * aliased to astro airflow * (#253)
- Change error messsage for astro deploy (#251)
- Remove active from Workspace type (#249)
- Relocate astro airflow deploy command (#233)
- Merge pull request #245 from astronomer/dead-link-in-comment-with
- Fix dead link in comment airflow_settings.yaml
- Merge pull request #243 from astronomer/index-out-of-range
- Replicate fixes from moby/moby repo for TERMINFO related bugfixes
- Update compose webserver link
- Merge pull request #241 from astronomer/hotfix/rbac-dashboard
- Fix link and output user and pass
- Merge pull request #235 from astronomer/hotfix-logiin-issue
- Hotfix: login issue
- Merge pull request #234 from astronomer/reimplement-docker-push
- Add explicit version in comment
- Return back Astro login
- Bump docker version
- Merge pull request #231 from astronomer/compose-override.yml
- Support "docker-compose.override.yml" files for local customization
- Merge pull request #222 from astronomer/enable-rbac-after-astro-airflow-init
- Fix create_user command
- Merge pull request #226 from astronomer/fix-issue-with-registry-auth-windows
- Hide create user logs
- Add create_user during astro airflow start
- Add more clean error message for start airflow container
- Update RunExample for astro airflow run
- Refactor using astro airflow run instead of hard code create_user
- Add docker exec to create default user for local airflow
- Add RBAC true by default locally
- Enable RBAC for local airflow
- Rename password to token
- Merge pull request #225 from astronomer/fix-issue-with-registry-auth-windows
- Remove example of login
- Add docker socket volume
- Rename test
- Fix Cannot unmarshal "" to type string into a string value
- Fix docker socket
- Change to privileged: true
- Update base image for unit/integration tests
- Fix how we are running unittests
- Change docker image to dnd
- Rething login to registry
- Merge pull request #223 from astronomer/add-success-message-after-deployment-completed
- Add success message after pushing all layers

## [0.8.x]

- Remove hard-coded deployment version
- Merge pull request #219 from astronomer/windows-astro-auth-login-issue
- Refactor using golang std lib
- Add new astro cli commands to support RBAC in Orbit (#214)
- remove unused comment
- Improve message texts
- Add Update Role
- Add add user with role to workspace
- Add `astro workspace user list`
- Fix linux/32bit/64bit settings: astro file not found (#216)
- Bump goreleaser (#215)
- Fix "gcc": executable file not found in $PATH (#213)
- Add newline to skip_pool message (#209)
- Add few notes how to run astro on Windows 10 (#207)
- Handle case with dashes in project name
- Handle case where pool description has spaces
- Add support for streaming logs (#196)
- Add gitHub pre-releases on alpha, beta, rc versions (#200)
- Add exact validator to workspace user add/remove (#199)
- Remove unclear message (#194)
- added more example to update deployment (#195)
- Fix workspace switch workflow (#188)

## [0.7.x]

- Add missing parent to menu link plugin (#187)
- Switch wording from UUID to ID (#184)
- Add possible fix of error handling (#185)
- Order deployment list alphabetically (#176)
- Set default airflow version to 1.10.2
- Guarantee unique project names (#175)
- Move airflow command back to settings.go
- Update missing airflow_settings message
- Add .gitignore
- Change settings.yaml to airflow_settings.yaml and add to Dockerignore
- Fix envFile error message
- Handle case where .env does not exist
- Handle case where settings.yaml does not exist
- Handle case where multiple containers with scheduler in name are running
- Move AirflowCommand to docker.go
- Move airflow command to AirflowCommand method
- Fix settings.yaml messages
- Fix drone pipeline
- Ensure we only show port when IP address is specified
- Handle cases where Pool Description and Slots are not set
- Make airflow command creations dynamic
- Feature/connections (#168)
- Change release name to deployment name (#157)
- Add ability to configure airflow version on init (#164)
- Fix Dockerfile padding
- Add --env as flag on start
- Add env_file to airflow configuration
- Add envFile to generateConfig
- Add .env file creation on init
- Update README.md
- Revert "Add --env as flag on start"
- Revert "Add env_file to airflow configuration"
- Revert "Add envFile to generateConfig"
- Revert "Add .env file creation on init"
- Add .env file creation on init
- Add envFile to generateConfig
- Add env_file to airflow configuration
- Add --env as flag on start
- Fix houston deployment version
- Merge pull request #158 from astronomer/docs-link
- Adding link to docs site
- Add deploy label and dynamicPadding (#155)
- Add slack notification to drone
