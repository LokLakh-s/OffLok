package org.loklakh.offlok

import android.app.Activity
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.view.View
import android.view.WindowInsets
import android.widget.FrameLayout
import android.webkit.JavascriptInterface
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import org.json.JSONObject
import java.io.FileNotFoundException

/**
 * OffLok's window: the computers' (desktop/frontend, in the app's assets) in a WebView, served from a made-up
 * HTTPS origin so that its modules load. Its api.js talks to the core through the OffLokAndroid bridge.
 */
class MainActivity : Activity() {
    companion object {
        private const val ORIGIN = "https://appassets.androidplatform.net"
        private const val ASK_VPN = 1
        private val types = mapOf(
            "html" to "text/html", "js" to "text/javascript", "css" to "text/css", "png" to "image/png",
            "svg" to "image/svg+xml", "ttf" to "font/ttf", "txt" to "text/plain",
        )
    }

    private lateinit var web: WebView
    private val waitingForVpn = mutableListOf<Int>() // the window's "on" calls, while Android asks the user

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Edge to edge on every version, as Android 15 makes it: the bars are transparent (the theme).
        if (Build.VERSION.SDK_INT >= 30) {
            window.setDecorFitsSystemWindows(false)
        } else {
            @Suppress("DEPRECATION")
            window.decorView.systemUiVisibility = View.SYSTEM_UI_FLAG_LAYOUT_STABLE or
                View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION or View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
        }
        web = WebView(this)
        web.setBackgroundColor(getColor(R.color.background))
        val root = FrameLayout(this)
        root.addView(web)
        // Edge to edge: the page keeps clear of the system bars and the display cutout.
        root.setOnApplyWindowInsetsListener { v, insets ->
            val bars = if (Build.VERSION.SDK_INT >= 30) {
                insets.getInsets(WindowInsets.Type.systemBars() or WindowInsets.Type.displayCutout()).let {
                    intArrayOf(it.left, it.top, it.right, it.bottom)
                }
            } else {
                @Suppress("DEPRECATION")
                intArrayOf(insets.systemWindowInsetLeft, insets.systemWindowInsetTop, insets.systemWindowInsetRight, insets.systemWindowInsetBottom)
            }
            v.setPadding(bars[0], bars[1], bars[2], bars[3])
            insets
        }
        web.settings.apply {
            javaScriptEnabled = true
            allowFileAccess = false
            allowContentAccess = false
            setSupportZoom(false)
        }
        web.webViewClient = object : WebViewClient() {
            override fun shouldInterceptRequest(view: WebView, request: WebResourceRequest): WebResourceResponse? {
                val url = request.url
                if ("${url.scheme}://${url.host}" != ORIGIN) return null
                val path = url.path.orEmpty().trimStart('/').ifEmpty { "index.html" }
                val type = types[path.substringAfterLast('.')] ?: "application/octet-stream"
                return try {
                    WebResourceResponse(type, "utf-8", assets.open(path))
                } catch (e: FileNotFoundException) {
                    WebResourceResponse("text/plain", "utf-8", 404, "Not Found", emptyMap(), "".byteInputStream())
                }
            }

            // Links out of the window (release notes, OffLok's site) open in the browser.
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                if (request.url.toString().startsWith("$ORIGIN/")) return false
                if (request.url.scheme == "https") startActivity(Intent(Intent.ACTION_VIEW, request.url))
                return true
            }
        }
        web.addJavascriptInterface(Bridge(), "OffLokAndroid")
        setContentView(root)
        web.loadUrl("$ORIGIN/index.html")
    }

    /** The window's calls. Each answers later, through window.offlokAnswer(id, result). */
    inner class Bridge {
        @JavascriptInterface
        fun call(id: Int, cmd: String, value: String) {
            if (cmd == "on") {
                VpnService.prepare(this@MainActivity)?.let { ask ->
                    runOnUiThread {
                        waitingForVpn += id
                        if (waitingForVpn.size == 1) startActivityForResult(ask, ASK_VPN)
                    }
                    return
                }
            }
            Thread { answer(id, Core.call(cmd, value)) }.start()
        }

        /** Opens one of Android's settings the window points to: "network" (Private DNS), "vpn". */
        @JavascriptInterface
        fun openSettings(which: String) {
            val action = if (which == "vpn") Settings.ACTION_VPN_SETTINGS else Settings.ACTION_WIRELESS_SETTINGS
            runOnUiThread { startActivity(Intent(action)) }
        }
    }

    @Deprecated("Activity's own; OffLok uses no AndroidX")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        if (requestCode != ASK_VPN) return super.onActivityResult(requestCode, resultCode, data)
        val ids = waitingForVpn.toList()
        waitingForVpn.clear()
        Thread {
            val result = if (resultCode == RESULT_OK) {
                Core.call("on")
            } else {
                JSONObject().put("ok", false).put("error", getString(R.string.vpn_refused)).toString()
            }
            ids.forEach { answer(it, result) }
        }.start()
    }

    private fun answer(id: Int, result: String) {
        runOnUiThread { web.evaluateJavascript("window.offlokAnswer($id, ${JSONObject.quote(result)})", null) }
    }

    override fun onResume() {
        super.onResume()
        web.evaluateJavascript("window.offlokRefresh && window.offlokRefresh()", null)
    }

    override fun onDestroy() {
        web.destroy()
        super.onDestroy()
    }
}
