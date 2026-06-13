plugins {
    id("com.android.application")
    id("kotlin-android")
    // The Flutter Gradle Plugin must be applied after the Android and Kotlin Gradle plugins.
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    namespace = "dev.smackerel.smackerel_assistant_example"
    compileSdk = flutter.compileSdkVersion
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = JavaVersion.VERSION_17.toString()
    }

    defaultConfig {
        // TODO: Specify your own unique Application ID (https://developer.android.com/studio/build/application-id.html).
        applicationId = "dev.smackerel.smackerel_assistant_example"
        // You can update the following values to match your application needs.
        // For more information, see: https://flutter.dev/to/review-gradle-config.
        minSdk = flutter.minSdkVersion
        targetSdk = flutter.targetSdkVersion
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    signingConfigs {
        // Spec 085 (FR-CBR-007) / knb spec 025 gate check (e): the Android
        // distribution upload keystore is OPERATOR-PRIVATE. Every credential is
        // read via System.getenv(...) — there is NO inline storePassword/
        // keyPassword literal and NO committed .jks/.keystore in this repo. CI
        // (the build-clients job in .github/workflows/build.yml) base64-decodes
        // the keystore from a GitHub secret into a runner-tmp path
        // (ANDROID_KEYSTORE_PATH) and supplies the three passwords as secrets;
        // the path is never committed and is removed at job end.
        create("release") {
            val keystorePath = System.getenv("ANDROID_KEYSTORE_PATH")
            if (keystorePath != null && keystorePath.isNotEmpty()) {
                storeFile = file(keystorePath)
                storePassword = System.getenv("ANDROID_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("ANDROID_KEY_ALIAS")
                keyPassword = System.getenv("ANDROID_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            // FR-CBR-007: when CI provides the operator-private upload keystore
            // (ANDROID_KEYSTORE_PATH + the three secrets, env-ref only) the
            // published release AAB/APK is distribution-signed with it (Lane A
            // sideload installable + Lane B Play-acceptable). For a local
            // `flutter run --release` (no secret present) Gradle uses the SDK
            // debug keystore so iteration still works — no inline literal and no
            // committed key material either way.
            signingConfig = if (System.getenv("ANDROID_KEYSTORE_PATH") != null) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
        }
    }
}

flutter {
    source = "../.."
}
