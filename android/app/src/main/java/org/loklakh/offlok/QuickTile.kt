package org.loklakh.offlok

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService

/** The Quick Settings tile: turns OffLok on and off, and shows its state. */
class QuickTile : TileService() {
    /**
     * Whether the user has the tile, as far as the app can tell (Android does not say), so that the window offers it
     * until then: "added", "dismissed" (the user said no), "ask" (Android 13 or later asks the user for the app) or
     * "steps" (the window says how to add it).
     */
    companion object {
        private const val PREFS = "tile"

        fun prompt(context: Context): String {
            val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            return when {
                prefs.getBoolean("added", false) -> "added"
                prefs.getBoolean("dismissed", false) -> "dismissed"
                Build.VERSION.SDK_INT >= 33 && !prefs.getBoolean("steps", false) -> "ask"
                else -> "steps"
            }
        }

        fun mark(context: Context, key: String, value: Boolean = true) {
            val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            if (prefs.getBoolean(key, false) != value) prefs.edit().putBoolean(key, value).apply()
        }
    }

    override fun onTileAdded() = mark(this, "added")

    override fun onTileRemoved() = mark(this, "added", false)

    // Android listens to a tile only while it is in Quick Settings: one added before the app kept track is there too.
    override fun onStartListening() {
        mark(this, "added")
        show()
    }

    override fun onClick() {
        if (!Core.isOn && VpnService.prepare(this) != null) {
            // Android asks for its permission once, from OffLok's window.
            openWindow()
            return
        }
        val cmd = if (Core.isOn) "off" else "on"
        Thread {
            Core.call(cmd)
            mainExecutor.execute { show() }
        }.start()
    }

    private fun show() {
        val tile = qsTile ?: return
        tile.state = if (Core.isOn) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
        if (Build.VERSION.SDK_INT >= 29) {
            tile.subtitle = getString(
                when (Core.state) {
                    "direct" -> R.string.tile_on
                    "fallback-dot", "fallback-network", "strict-blocked", "offline" -> R.string.tile_fallback
                    "error" -> R.string.tile_error
                    else -> R.string.tile_off
                },
            )
        }
        tile.updateTile()
    }

    private fun openWindow() {
        val intent = Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        if (Build.VERSION.SDK_INT >= 34) {
            startActivityAndCollapse(PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_IMMUTABLE))
        } else {
            @Suppress("DEPRECATION")
            startActivityAndCollapse(intent)
        }
    }
}
