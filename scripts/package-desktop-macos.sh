#!/usr/bin/env bash

set -euo pipefail

VERSION="${1:-0.0.0}"
PLIST_VERSION="${VERSION#v}"
APP_NAME="baize"
APP_DISPLAY_NAME="baize"
BUNDLE_ID="${MACOS_BUNDLE_ID:-com.baize.desktop}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
BUILD_DIR="${ROOT_DIR}/cmd/baize-desktop/build/bin"
APP_SRC="${BUILD_DIR}/${APP_NAME}.app"
APP_BINARY="${APP_SRC}/Contents/MacOS/${APP_NAME}"
APP_RESOURCES_DIR="${APP_SRC}/Contents/Resources"
APP_ICON_SRC="${ROOT_DIR}/cmd/baize-desktop/build/appicon.png"
APP_ICON_NAME="appicon.icns"
APP_ICON_FILE="${APP_RESOURCES_DIR}/${APP_ICON_NAME}"
DMG_PATH="${DIST_DIR}/${APP_NAME}-macos-${VERSION}.dmg"
APP_ZIP="${DIST_DIR}/${APP_NAME}-macos-${VERSION}.zip"
BUILD_PLATFORM="${MACOS_BUILD_PLATFORM:-darwin/arm64}"
GO_BIN="${GO_BIN:-$(command -v go)}"
SDK_PATH="${SDK_PATH:-$(xcrun --sdk macosx --show-sdk-path)}"
GO_TAGS="${GO_TAGS:-desktop,wv2runtime.download,production}"
GO_LDFLAGS="${GO_LDFLAGS:--w -s -extldflags '-framework UniformTypeIdentifiers'}"
APP_VERSION_LDFLAG="-X main.appVersion=${VERSION}"

TMP_DIR=""
ICON_TMP_DIR=""
CERT_PATH=""
KEYCHAIN_PATH=""
KEYCHAIN_PASSWORD=""
ORIGINAL_DEFAULT_KEYCHAIN=""
declare -a ORIGINAL_KEYCHAINS=()

cleanup() {
    if [[ ${#ORIGINAL_KEYCHAINS[@]} -gt 0 ]]; then
        security list-keychains -d user -s "${ORIGINAL_KEYCHAINS[@]}" >/dev/null 2>&1 || true
    fi

    if [[ -n "${ORIGINAL_DEFAULT_KEYCHAIN}" ]]; then
        security default-keychain -d user -s "${ORIGINAL_DEFAULT_KEYCHAIN}" >/dev/null 2>&1 || true
    fi

    if [[ -n "${KEYCHAIN_PATH}" ]]; then
        security delete-keychain "${KEYCHAIN_PATH}" >/dev/null 2>&1 || true
    fi

    [[ -n "${CERT_PATH}" ]] && rm -f "${CERT_PATH}"
    [[ -n "${APP_ZIP}" ]] && rm -f "${APP_ZIP}"
    [[ -n "${ICON_TMP_DIR}" ]] && rm -rf "${ICON_TMP_DIR}"
    [[ -n "${TMP_DIR}" ]] && rm -rf "${TMP_DIR}"
}

require_complete_signing_env() {
    if [[ -n "${MAC_CERT_P12_BASE64:-}" || -n "${MAC_CERT_PASSWORD:-}" ]]; then
        if [[ -z "${MAC_CERT_P12_BASE64:-}" || -z "${MAC_CERT_PASSWORD:-}" ]]; then
            echo "MAC_CERT_P12_BASE64 and MAC_CERT_PASSWORD must be set together." >&2
            exit 1
        fi
    fi

    if [[ -n "${APPLE_ID:-}" || -n "${APPLE_APP_PASSWORD:-}" || -n "${APPLE_TEAM_ID:-}" ]]; then
        if [[ -z "${APPLE_ID:-}" || -z "${APPLE_APP_PASSWORD:-}" || -z "${APPLE_TEAM_ID:-}" ]]; then
            echo "APPLE_ID, APPLE_APP_PASSWORD, and APPLE_TEAM_ID must be set together." >&2
            exit 1
        fi
    fi
}

import_signing_certificate() {
    CERT_PATH="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/${APP_NAME}.p12"
    KEYCHAIN_PATH="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/${APP_NAME}.keychain-db"
    KEYCHAIN_PASSWORD="${KEYCHAIN_PASSWORD:-$(uuidgen)}"

    CERT_PATH="${CERT_PATH}" python3 -c 'import base64, os, pathlib; pathlib.Path(os.environ["CERT_PATH"]).write_bytes(base64.b64decode(os.environ["MAC_CERT_P12_BASE64"]))'

    ORIGINAL_KEYCHAINS=()
    while IFS= read -r keychain; do
        ORIGINAL_KEYCHAINS+=("${keychain}")
    done < <(security list-keychains -d user | sed 's/^[[:space:]]*//; s/^"//; s/"$//')
    ORIGINAL_DEFAULT_KEYCHAIN="$(security default-keychain -d user | sed 's/^[[:space:]]*//; s/^"//; s/"$//')"

    rm -f "${KEYCHAIN_PATH}"
    security create-keychain -p "${KEYCHAIN_PASSWORD}" "${KEYCHAIN_PATH}"
    security set-keychain-settings -lut 21600 "${KEYCHAIN_PATH}"
    security unlock-keychain -p "${KEYCHAIN_PASSWORD}" "${KEYCHAIN_PATH}"
    security import "${CERT_PATH}" \
        -k "${KEYCHAIN_PATH}" \
        -P "${MAC_CERT_PASSWORD}" \
        -T /usr/bin/codesign \
        -T /usr/bin/security \
        -T /usr/bin/xcrun
    security list-keychains -d user -s "${KEYCHAIN_PATH}" "${ORIGINAL_KEYCHAINS[@]}"
    security default-keychain -d user -s "${KEYCHAIN_PATH}"
    security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "${KEYCHAIN_PASSWORD}" "${KEYCHAIN_PATH}"

    if [[ -z "${CODESIGN_IDENTITY:-}" ]]; then
        CODESIGN_IDENTITY="$(security find-identity -v -p codesigning "${KEYCHAIN_PATH}" | awk -F'"' 'NF >= 2 { print $2; exit }')"
    fi

    if [[ -z "${CODESIGN_IDENTITY:-}" ]]; then
        echo "Unable to find a codesigning identity after importing the certificate." >&2
        exit 1
    fi
}

compile_binary() {
    local goarch="$1"
    local output_path="$2"
    local clang_arch="$goarch"

    if [[ "${goarch}" == "amd64" ]]; then
        clang_arch="x86_64"
    fi

    env \
        GOOS=darwin \
        GOARCH="${goarch}" \
        CGO_ENABLED=1 \
        CC=/usr/bin/clang \
        CXX=/usr/bin/clang++ \
        SDKROOT="${SDK_PATH}" \
        CGO_CFLAGS="-arch ${clang_arch}" \
        CGO_CXXFLAGS="-arch ${clang_arch}" \
        CGO_LDFLAGS="-arch ${clang_arch} -framework UniformTypeIdentifiers" \
        "${GO_BIN}" build \
            -buildvcs=false \
            -tags "${GO_TAGS}" \
            -ldflags "${GO_LDFLAGS} ${APP_VERSION_LDFLAG}" \
            -o "${output_path}" \
            ./cmd/baize-desktop
}

build_binary() {
    local universal_binary=""
    local amd64_binary=""
    local arm64_binary=""

    mkdir -p "${DIST_DIR}" "${BUILD_DIR}"
    rm -f "${DMG_PATH}" "${APP_ZIP}"

    case "${BUILD_PLATFORM}" in
        darwin/arm64)
            compile_binary arm64 "${APP_BINARY}"
            ;;
        darwin/amd64)
            compile_binary amd64 "${APP_BINARY}"
            ;;
        darwin/universal)
            universal_binary="$(mktemp "${TMPDIR:-/tmp}/${APP_NAME}-universal.XXXXXX")"
            amd64_binary="$(mktemp "${TMPDIR:-/tmp}/${APP_NAME}-amd64.XXXXXX")"
            arm64_binary="$(mktemp "${TMPDIR:-/tmp}/${APP_NAME}-arm64.XXXXXX")"
            compile_binary amd64 "${amd64_binary}"
            compile_binary arm64 "${arm64_binary}"
            lipo -create -output "${universal_binary}" "${amd64_binary}" "${arm64_binary}"
            cp "${universal_binary}" "${APP_BINARY}"
            rm -f "${universal_binary}" "${amd64_binary}" "${arm64_binary}"
            ;;
        *)
            echo "Unsupported MACOS_BUILD_PLATFORM: ${BUILD_PLATFORM}" >&2
            exit 1
            ;;
    esac

    chmod +x "${APP_BINARY}"
}

render_icon_png() {
    local size="$1"
    local output_path="$2"

    sips -s format png -z "${size}" "${size}" "${APP_ICON_SRC}" --out "${output_path}" >/dev/null
}

create_app_icon() {
    local icon_tiff=""
    local iconset_dir=""
    local -a icon_pngs=()

    if [[ ! -f "${APP_ICON_SRC}" ]]; then
        echo "Missing macOS app icon source: ${APP_ICON_SRC}" >&2
        exit 1
    fi

    ICON_TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/${APP_NAME}-iconset.XXXXXX")"
    iconset_dir="${ICON_TMP_DIR}/png"
    icon_tiff="${ICON_TMP_DIR}/${APP_ICON_NAME%.icns}.tiff"
    mkdir -p "${iconset_dir}"

    render_icon_png 16 "${iconset_dir}/icon_16x16.png"
    render_icon_png 32 "${iconset_dir}/icon_32x32.png"
    render_icon_png 48 "${iconset_dir}/icon_48x48.png"
    render_icon_png 128 "${iconset_dir}/icon_128x128.png"
    render_icon_png 256 "${iconset_dir}/icon_256x256.png"
    render_icon_png 512 "${iconset_dir}/icon_512x512.png"
    render_icon_png 1024 "${iconset_dir}/icon_1024x1024.png"

    icon_pngs=(
        "${iconset_dir}/icon_16x16.png"
        "${iconset_dir}/icon_32x32.png"
        "${iconset_dir}/icon_48x48.png"
        "${iconset_dir}/icon_128x128.png"
        "${iconset_dir}/icon_256x256.png"
        "${iconset_dir}/icon_512x512.png"
        "${iconset_dir}/icon_1024x1024.png"
    )

    tiffutil -catnosizecheck "${icon_pngs[@]}" -out "${icon_tiff}" >/dev/null
    tiff2icns "${icon_tiff}" "${APP_ICON_FILE}" >/dev/null
}

write_info_plist() {
    cat > "${APP_SRC}/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>CFBundleDisplayName</key>
    <string>${APP_DISPLAY_NAME}</string>
    <key>CFBundleExecutable</key>
    <string>${APP_NAME}</string>
    <key>CFBundleIdentifier</key>
    <string>${BUNDLE_ID}</string>
    <key>CFBundleIconFile</key>
    <string>${APP_ICON_NAME}</string>
    <key>CFBundleName</key>
    <string>${APP_DISPLAY_NAME}</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>${PLIST_VERSION}</string>
    <key>CFBundleVersion</key>
    <string>${PLIST_VERSION}</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.13.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
  </dict>
</plist>
EOF
}

create_app_bundle() {
    rm -rf "${APP_SRC}"
    mkdir -p "${APP_SRC}/Contents/MacOS" "${APP_RESOURCES_DIR}"
    build_binary
    create_app_icon
    write_info_plist
    printf 'APPL????' > "${APP_SRC}/Contents/PkgInfo"
}

sign_app() {
    xattr -cr "${APP_SRC}" || true
    codesign --force --deep --options runtime --timestamp --sign "${CODESIGN_IDENTITY}" "${APP_SRC}"
    codesign --verify --deep --strict --verbose=2 "${APP_SRC}"
}

sign_dmg() {
    codesign --force --timestamp --sign "${CODESIGN_IDENTITY}" "${DMG_PATH}"
    codesign --verify --verbose=2 "${DMG_PATH}"
}

notarize_artifact() {
    local artifact="$1"

    xcrun notarytool submit "${artifact}" \
        --apple-id "${APPLE_ID}" \
        --password "${APPLE_APP_PASSWORD}" \
        --team-id "${APPLE_TEAM_ID}" \
        --wait
}

zip_app_for_notary() {
    ditto -c -k --sequesterRsrc --keepParent "${APP_SRC}" "${APP_ZIP}"
}

create_dmg() {
    TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/${APP_NAME}-dmg.XXXXXX")"
    cp -R "${APP_SRC}" "${TMP_DIR}/"
    ln -s /Applications "${TMP_DIR}/Applications"

    hdiutil create \
        -volname "baize" \
        -srcfolder "${TMP_DIR}" \
        -ov \
        -format UDZO \
        "${DMG_PATH}"
}

trap cleanup EXIT

require_complete_signing_env
create_app_bundle

SHOULD_SIGN="false"
SHOULD_NOTARIZE="false"

if [[ -n "${CODESIGN_IDENTITY:-}" || -n "${MAC_CERT_P12_BASE64:-}" ]]; then
    SHOULD_SIGN="true"
fi

if [[ -n "${APPLE_ID:-}" ]]; then
    SHOULD_NOTARIZE="true"
fi

if [[ "${SHOULD_NOTARIZE}" == "true" && "${SHOULD_SIGN}" != "true" ]]; then
    echo "Apple notarization requires a signed app. Provide a certificate or a local codesign identity." >&2
    exit 1
fi

if [[ -n "${MAC_CERT_P12_BASE64:-}" ]]; then
    import_signing_certificate
fi

if [[ "${SHOULD_SIGN}" == "true" ]]; then
    sign_app
fi

if [[ "${SHOULD_NOTARIZE}" == "true" ]]; then
    zip_app_for_notary
    notarize_artifact "${APP_ZIP}"
    xcrun stapler staple "${APP_SRC}"
    xcrun stapler validate "${APP_SRC}"
fi

create_dmg

if [[ "${SHOULD_SIGN}" == "true" ]]; then
    sign_dmg
fi

if [[ "${SHOULD_NOTARIZE}" == "true" ]]; then
    notarize_artifact "${DMG_PATH}"
    xcrun stapler staple "${DMG_PATH}"
    xcrun stapler validate "${DMG_PATH}"
fi

echo "Created ${DMG_PATH}"
