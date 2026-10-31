# The Go core's Java side (gomobile): Go calls it through JNI, by name.
-keep class go.** { *; }
-keep class org.loklakh.offlok.mobile.** { *; }
-keep class * implements org.loklakh.offlok.mobile.Platform { *; }
# The window's bridge.
-keepclassmembers class * {
    @android.webkit.JavascriptInterface <methods>;
}
