# Changelog

All notable changes to this project will be documented in this file.

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
