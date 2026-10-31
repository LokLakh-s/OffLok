package org.loklakh.offlok

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent

/** The VPN service's notification: OffLok's state, as the computers' tray icon shows it. */
object Notifications {
    private const val CHANNEL = "status"
    private const val ID = 1

    fun createChannel(context: Context) {
        val channel = NotificationChannel(CHANNEL, context.getString(R.string.channel_status), NotificationManager.IMPORTANCE_LOW)
        channel.description = context.getString(R.string.channel_status_text)
        channel.setShowBadge(false)
        context.getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
    }

    fun build(context: Context): Notification {
        val window = PendingIntent.getActivity(context, 0, Intent(context, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        return Notification.Builder(context, CHANNEL)
            .setSmallIcon(R.drawable.ic_notification)
            .setColor(context.getColor(R.color.teal))
            .setContentTitle(context.getString(stateTitle(Core.state)))
            .setContentIntent(window)
            .setOngoing(true)
            .setShowWhen(false)
            .setCategory(Notification.CATEGORY_SERVICE)
            .build()
    }

    fun show(context: Context) {
        context.getSystemService(NotificationManager::class.java).notify(ID, build(context))
    }

    fun stateTitle(state: String): Int = when (state) {
        "direct" -> R.string.state_direct
        "fallback-dot" -> R.string.state_fallback_dot
        "fallback-network" -> R.string.state_fallback_network
        "strict-blocked" -> R.string.state_strict_blocked
        "offline" -> R.string.state_offline
        "off" -> R.string.state_off
        "error" -> R.string.state_error
        else -> R.string.state_starting
    }
}
