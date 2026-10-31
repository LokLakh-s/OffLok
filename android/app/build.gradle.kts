import java.util.Properties

plugins {
    id("com.android.application")
}

// The pinned build tools (packaging/android/android.env, which the scripts read too) and OffLok's version.
val env = Properties().apply {
    rootProject.file("../packaging/android/android.env").readLines()
        .filter { Regex("^[A-Z_]+=.*").matches(it) }
        .forEach { line -> line.split("=", limit = 2).let { setProperty(it[0], it[1].trim('"')) } }
}
val offlokVersion = rootProject.file("../VERSION").readText().trim()
val (major, minor, patch) = offlokVersion.split(".").map { it.toInt() }

android {
    namespace = "org.loklakh.offlok"
    compileSdk = env.getProperty("ANDROID_TARGET_API").toInt()
    buildToolsVersion = env.getProperty("ANDROID_BUILD_TOOLS")
    ndkVersion = env.getProperty("NDK_VERSION")

    defaultConfig {
        applicationId = "org.loklakh.offlok"
        minSdk = env.getProperty("ANDROID_MIN_API").toInt()
        targetSdk = env.getProperty("ANDROID_TARGET_API").toInt()
        versionCode = major * 10000 + minor * 100 + patch
        versionName = offlokVersion
    }

    // Google Play's app blocks no ads (its VpnService policy); F-Droid's and GitHub's are the full app.
    flavorDimensions += "store"
    productFlavors {
        create("full") { dimension = "store" }
        create("play") { dimension = "store" }
    }

    // The release key, when the build has it (OFFLOK_KEYSTORE...): the same for GitHub, F-Droid and Google Play.
    val keystore = System.getenv("OFFLOK_KEYSTORE")
    if (keystore != null) {
        signingConfigs.create("release") {
            storeFile = file(keystore)
            storePassword = System.getenv("OFFLOK_KEYSTORE_PASSWORD")
            keyAlias = System.getenv("OFFLOK_KEY_ALIAS")
            keyPassword = System.getenv("OFFLOK_KEY_PASSWORD")
        }
    }
    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            if (keystore != null) signingConfig = signingConfigs.getByName("release")
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    // The window is the computers' (desktop/frontend), and the app shows Unbound's and OpenSSL's licences.
    sourceSets.getByName("main") {
        assets.srcDirs("../../desktop/frontend", "../../internal/libunbound/_native/licences")
    }
    // F-Droid reads no blob Google signs: no dependency list in the APK.
    dependenciesInfo {
        includeInApk = false
        includeInBundle = false
    }
    buildFeatures { buildConfig = true }
}

dependencies {
    "fullImplementation"(files("libs/offlok-core-full.aar"))
    "playImplementation"(files("libs/offlok-core-play.aar"))
}
