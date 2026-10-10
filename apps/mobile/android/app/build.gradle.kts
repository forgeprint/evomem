plugins {
    id("com.android.application")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    // One identity across both platforms: this matches CFBundleIdentifier in
    // ios/Runner/Info.plist and PRODUCT_BUNDLE_IDENTIFIER in the Xcode
    // project. The generated default was com.evomem.evomem_mobile, which
    // would have shipped two different identities for the same app.
    namespace = "com.evomem.mobile"
    compileSdk = flutter.compileSdkVersion
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    defaultConfig {
        applicationId = "com.evomem.mobile"

        // Pinned, not inherited from flutter.minSdkVersion. Two plugins in
        // this build set minSdk 24 in their own android/build.gradle, read
        // from the pub cache on 2026-10-10: record_android 2.2.0 (line 39)
        // and flutter_secure_storage 11.2.0 (line 30). Gradle refuses a
        // library floor above the application's, so a lower number here is
        // not a warning, it is a build failure.
        //
        // flutter.minSdkVersion happens to be 24 today. Inheriting it would
        // mean a Flutter upgrade that lowered the default silently broke the
        // build instead of changing a number somebody chose.
        minSdk = 24
        targetSdk = flutter.targetSdkVersion
        // Uses the version code from pubspec.yaml. When using split APKs, 1000 * ABI_VERSION
        // is added automatically by Flutter. (https://developer.android.com/studio/build/configure-apk-splits#configure-APK-versions)
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    buildTypes {
        release {
            // Signed with the debug keys, so `flutter run --release` works.
            // A release that goes anywhere needs its own signing config and
            // a keystore that is not in this repository; there is no release
            // channel for the phone app yet, so there is nothing to sign.
            signingConfig = signingConfigs.getByName("debug")
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget = org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17
    }
}

flutter {
    source = "../.."
}
