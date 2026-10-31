package org.loklakh.offlok

import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/**
 * The VPN that carries the phone's DNS queries to OffLok, and nothing else: its DNS servers are OffLok's addresses,
 * which are also its only routes. OffLok itself is left out of it, so that it sees the network the phone uses. The
 * core opens it (Platform.openTunnel) and reads it; the service holds it and shows OffLok's state while it runs.
 */
class OffLokVpnService : VpnService() {
    companion object {
        // OffLok's addresses in the tunnel (mobile/backend.go), and the phone's.
        private const val DNS4 = "10.111.222.1"
        private const val DNS6 = "fd4f:666c:6f6b::1"
        private const val PHONE4 = "10.111.222.2"
        private const val PHONE6 = "fd4f:666c:6f6b::2"
        // Large enough for any DNS answer over UDP: the core never truncates one.
        private const val MTU = 65535
        private const val NOTIFICATION = 1

        @Volatile
        private var running: OffLokVpnService? = null
        @Volatile
        private var started = CountDownLatch(1)

        /** Starts the service if it is not running, then the tunnel; returns its file descriptor, detached. */
        fun open(context: Context): Long {
            check(VpnService.prepare(context) == null) { "Android has not let OffLok start a VPN: turn OffLok on from its window" }
            if (running == null) {
                started = CountDownLatch(1)
                context.startForegroundService(Intent(context, OffLokVpnService::class.java))
                started.await(10, TimeUnit.SECONDS)
            }
            val service = checkNotNull(running) { "OffLok's VPN service did not start" }
            return service.establishTunnel()
        }

        /** The core closed the tunnel: the service ends. */
        fun close() {
            running?.let {
                it.stopForeground(Service.STOP_FOREGROUND_REMOVE)
                it.stopSelf()
            }
        }

        fun showState(context: Context) {
            running?.let { Notifications.show(it) }
        }
    }

    override fun onCreate() {
        super.onCreate()
        running = this
        started.countDown()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val notification = Notifications.build(this)
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(NOTIFICATION, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_SYSTEM_EXEMPTED)
        } else {
            startForeground(NOTIFICATION, notification)
        }
        // Android's "Always-on VPN" starts the service itself, at boot: the user wants OffLok on.
        if (intent?.action == VpnService.SERVICE_INTERFACE) {
            Thread { Core.call("on") }.start()
        }
        return START_NOT_STICKY
    }

    private fun establishTunnel(): Long {
        val window = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        val builder = Builder()
            .setSession(getString(R.string.app_name))
            .setConfigureIntent(window)
            .setMtu(MTU)
            .addAddress(PHONE4, 32)
            .addAddress(PHONE6, 128)
            .addDnsServer(DNS4)
            .addDnsServer(DNS6)
            .addRoute(DNS4, 32)
            .addRoute(DNS6, 128)
            .addDisallowedApplication(packageName)
        if (Build.VERSION.SDK_INT >= 29) {
            // The tunnel carries no traffic: whether the network is metered is the underlying network's say.
            builder.setMetered(false)
        }
        val tunnel = checkNotNull(builder.establish()) { "Android refused OffLok's VPN" }
        return tunnel.detachFd().toLong()
    }

    /** Android took the VPN from OffLok: another VPN app started, or the user turned OffLok's off in Android's settings. */
    override fun onRevoke() {
        Thread { org.loklakh.offlok.mobile.Mobile.revoked() }.start()
        super.onRevoke()
    }

    override fun onDestroy() {
        if (running === this) running = null
        super.onDestroy()
    }
}
