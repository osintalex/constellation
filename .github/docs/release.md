# Release Checklist

This checklist will prepare `v1.3.0` from `v1.2.0`. Adjust your version numbers accordingly.

1. Merge ready PRs
2. Search the code for TODOs and FIXMEs that should be resolved before releasing.
3. Create docs release (new major or minor release)

    ```sh
    cd docs
    npm install
    npm run docusaurus docs:version 1.3
    # push upstream via PR
    ```

4. Create a new branch `release/v1.3` (new minor version) or use the existing one (new patch version)
5. On this branch, prepare the following things:
    1. (new patch version) `cherry-pick` (only) the required commits from `main`
    2. Use [Build micro-service manual](https://github.com/edgelesssys/constellation/actions/workflows/build-micro-service-manual.yml) and run the pipeline once for each micro-service with the following parameters:
        * branch: `release/v1.3`
        * Container image tag: `v1.3.0`
        * Version of the image to build: `1.3.0`

       ```sh
       ver=1.3.0
       ```

        ```sh
        minor=$(echo $ver | cut -d '.' -f 1,2)
        gcpVer=$(echo $ver | tr "." "-")
        echo $minor # should be 1.3
        echo $gcpVer # should be 1-3-0
        ```

        ```sh
        gh workflow run build-micro-service-manual.yml --ref release/v$minor -F microService=join-service -F imageTag=v$ver -F version=$ver --repo edgelesssys/constellation
        gh workflow run build-micro-service-manual.yml --ref release/v$minor -F microService=kmsserver -F imageTag=v$ver -F version=$ver --repo edgelesssys/constellation
        gh workflow run build-micro-service-manual.yml --ref release/v$minor -F microService=verification-service -F imageTag=v$ver -F version=$ver --repo edgelesssys/constellation
        ```

    3. Use [Build operator manual](https://github.com/edgelesssys/constellation/actions/workflows/build-operator-manual.yml) and run the pipeline once with the following parameters:
        * branch: `release/v1.3`
        * Container image tag: `v1.3.0`

        ```sh
        # Alternative from CLI
        gh workflow run build-operator-manual.yml --ref release/v$minor -F imageTag=v$ver --repo edgelesssys/constellation
        ```

    4. Review and update changelog with all changes since last release. [GitHub's diff view](https://github.com/edgelesssys/constellation/compare/v2.0.0...main) helps a lot!
       1. Rename the "Unreleased" heading to "[v1.3.0] - YYYY-MM-DD" and link the version to the upcoming release tag.
       2. Create a new block for unreleased changes
    5. Update project version in [CMakeLists.txt](/CMakeLists.txt) to `1.3.0` (without v).
    6. Update the `version` key in [constellation-services/Chart.yaml](/cli/internal/helm/charts/edgeless/constellation-services/Chart.yaml). Also update the `version` key for all subcharts, e.g. [Chart.yaml](/cli/internal/helm/charts/edgeless/constellation-services/charts/kms/Chart.yaml). Lastly, update the `dependencies.*.version` key for all dependencies in the main chart [constellation-services/Chart.yaml](/cli/internal/helm/charts/edgeless/constellation-services/Chart.yaml).
    7. Update [default image versions in enterprise config](/internal/config/images_enterprise.go)
    8. Increase version number of QEMU image `ConstellationQEMUImageURL` in [versions.go](../../internal/versions/versions.go#L64)
    9. When the microservice builds are finished update versions in [versions.go](../../internal/versions/versions.go#L33-L39) to `v1.3.0`, **add the container hashes** and **push your changes**.

        ```sh
        # crane: https://github.com/google/go-containerregistry/blob/main/cmd/crane/doc/crane.md
        crane digest ghcr.io/edgelesssys/constellation/node-operator-catalog:v$ver
        crane digest ghcr.io/edgelesssys/constellation/join-service:v$ver
        crane digest ghcr.io/edgelesssys/constellation/access-manager:v$ver
        crane digest ghcr.io/edgelesssys/constellation/kmsserver:v$ver
        crane digest ghcr.io/edgelesssys/constellation/verification-service:v$ver
        ```

    10. Create a [production OS image](/.github/workflows/build-os-image.yml)

        ```sh
        gh workflow run build-os-image.yml --ref release/v$minor -F debug=false -F imageVersion=v$ver
        ```

        * Once the pipeline has finished, download the artifact `image-qemu`.
        * Unzip the downloaded artifact, rename it to `constellation.raw`.
        * Go to the [S3 bucket for QEMU images](https://s3.console.aws.amazon.com/s3/buckets/cdn-constellation-backend?region=eu-central-1&prefix=constellation/images/mini-constellation/&showversions=false)
        * Create a new folder for the given version, and upload `constellation.raw` into it.

        * Replace AWS AMIs for this version and next in docs in `first-steps.md`.

    11. Run manual E2E tests using [Linux](/.github/workflows/e2e-test-manual.yml) and [macOS](/.github/workflows/e2e-test-manual-macos.yml) to confirm functionality and stability.

        ```sh
        gh workflow run e2e-test-manual.yml --ref release/v$minor -F cloudProvider=azure -F machineType=Standard_DC4as_v5 -F test="sonobuoy full" -F osImage=v$ver -F isDebugImage=false
        gh workflow run e2e-test-manual-macos.yml --ref release/v$minor -F cloudProvider=azure -F machineType=Standard_DC4as_v5 -F test="sonobuoy full" -F osImage=v$ver -F isDebugImage=false
        gh workflow run e2e-test-manual.yml --ref release/v$minor -F cloudProvider=gcp -F machineType=n2d-standard-4 -F test="sonobuoy full" -F osImage=v$ver -F isDebugImage=false
        gh workflow run e2e-test-manual-macos.yml --ref release/v$minor -F cloudProvider=gcp -F machineType=n2d-standard-4 -F test="sonobuoy full" -F osImage=v$ver -F isDebugImage=false
        ```

    12. [Generate measurements](/.github/workflows/generate-measurements.yml) for the images on each CSP.

        ```sh
           gh workflow run generate-measurements.yml --ref release/v$minor -F cloudProvider=azure -F osImage=v$ver -F isDebugImage=false
           gh workflow run generate-measurements.yml --ref release/v$minor -F cloudProvider=gcp -F osImage=v$ver -F isDebugImage=false
        ```

    13. Create a new tag on this release branch

        ```sh
        git tag v$ver
        git tags --push
        ```

        * Run [Release CLI](https://github.com/edgelesssys/constellation/actions/workflows/release-cli.yml) action on the tag

        ```sh
        gh workflow run release-cli.yml --ref v$ver
        ```

        * The previous step will create a draft release. Check build output for link to draft release. Review & approve.
6. Export, download and make image available in S3 for trusted launch users. To achieve this:

    ```sh
    TARGET_DISK=export-${ver}
    az disk create -g constellation-images -l westus -n ${TARGET_DISK} --hyper-v-generation V2 --os-type Linux --sku standard_lrs --security-type TrustedLaunch --gallery-image-reference /subscriptions/0d202bbb-4fa7-4af8-8125-58c269a05435/resourceGroups/CONSTELLATION-IMAGES/providers/Microsoft.Compute/galleries/Constellation/images/constellation/versions/${ver}
    ```

    * Find the created resource in Azure
    * Go to `Settings` -> `Export` and `Generate URLs`
    * Download both the disk image (first link) and VM state (second link)
    * Rename disk (`abcd`) to `constellation.img`.
    * Rename state (UUID) to `constellation.vmgs`.
    * Go to [AWS S3 bucket for trusted launch](https://s3.console.aws.amazon.com/s3/buckets/cdn-constellation-backend?prefix=constellation/images/azure/trusted-launch/&region=eu-central-1), create a new folder with the given version number.
    * Upload both image and state into the newly created folder.
    * Delete the disk in Azure!

7. To bring updated version numbers and other changes (if any) to main, create a new branch `feat/release` from `release/v1.3`, rebase it onto main, and create a PR to main
8. Milestones management
   1. Create a new milestone for the next release
   2. Add the next release manager and an approximate release date to the milestone description
   3. Close the milestone for the release
   4. Move open issues and PRs from closed milestone to next milestone
9. If the release is a minor version release, create an empty commit on main and tag it as the start of the next pre-release phase.

    ```sh
    nextMinorVer=$(echo $ver | awk -F. -v OFS=. '{$2 += 1 ; print}')
    git checkout main
    git pull
    git commit --allow-empty -m "Start v$nextMinorVer-pre"
    git tag v$nextMinorVer-pre
    git push origin main v$nextMinorVer-pre
    ```

10. Test Constellation mini up

11. Upload AWS measurements to S3 bucket:
    * Create an AWS cluster using the released version.
    * Use `hack/pcr-reader` to download measurements.
    * Create a new folder named after each AWS AMI in [S3 public bucket](https://s3.console.aws.amazon.com/s3/buckets/public-edgeless-constellation?region=us-east-2&tab=objects).
    * Keep measurements: 4, 8, 9, 11, 12, 13.
    * Sign the measurements using `cosign sign-blob`.
    * Upload both `measurements.yaml` & `measurements.yaml.sig` to each created folder in S3.
