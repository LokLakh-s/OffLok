package org.loklakh.offlok

import android.content.ComponentName
import android.content.Context
import android.service.quicksettings.TileService
import android.util.Log
import org.json.JSONObject
import org.loklakh.offlok.mobile.Mobile
import org.loklakh.offlok.mobile.Platform

/** OffLok's Go core (the mobile package, built with gomobile), started once per process. */
object Core : Platform {
    private const val TAG = "OffLok"
    private lateinit var app: Context

    /** The core's last status: what the notification and the Quick Settings tile show. */
    @Volatile
    var status: JSONObject = JSONObject()
        private set

    val state: String get() = status.optString("state", "starting")
    val isOn: Boolean get() = status.optJSONObject("settings")?.optBoolean("enabled") == true

    @Synchronized
    fun start(context: Context) {
        if (::app.isInitialized) return
        app = context.applicationContext
        Notifications.createChannel(app)
        Mobile.start(app.filesDir.absolutePath, BuildConfig.VERSION_NAME, this)
        NetworkWatch.start(app)
        refresh()
    }

    /** Runs one of the core's commands, as the window sends them; the result is JSON. Never on the main thread. */
    fun call(cmd: String, value: String = ""): String {
        val result = Mobile.call(cmd, value)
        JSONObject(result).optJSONObject("status")?.let { update(it) }
        return result
    }

    fun refresh() {
        call("status")
    }

    private fun update(st: JSONObject) {
        val before = state
        status = st
        if (before != state) {
            OffLokVpnService.showState(app)
            TileService.requestListeningState(app, ComponentName(app, QuickTile::class.java))
        }
    }

    // Platform: what the core asks of the app.

    override fun openTunnel(): Long = OffLokVpnService.open(app)

    override fun tunnelClosed() = OffLokVpnService.close()

    override fun changed(status: String) = update(JSONObject(status))

    override fun log(line: String) {
        Log.i(TAG, line)
    }
}
