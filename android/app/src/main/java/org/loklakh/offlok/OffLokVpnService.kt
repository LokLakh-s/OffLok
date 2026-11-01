package org.loklakh.offlok

import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.net.VpnService
import android.os.Binder
import android.os.Build
import android.os.IBinder
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/**
 * The VPN that carries the phone's DNS queries to OffLok, and nothing else: its DNS servers are OffLok's addresses,
 * which are also its only routes. OffLok itself is left out of it, so that it sees the network the phone uses. The
 * core opens it (Platform.openTunnel) and reads it.
 *
 * No notification: the app binds the service to open the tunnel (from the window, the tile or a restart alike), and
 * once the VPN is up, Android binds it too and keeps the app running; its key icon shows the VPN, the window and
 * the tile show OffLok's state.
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
        // The app's own binding; Android's comes with VpnService.SERVICE_INTERFACE.
        private const val BIND_APP = "org.loklakh.offlok.BIND_APP"

        @Volatile
        private var service: OffLokVpnService? = null
        private var connection: ServiceConnection? = null

        /** Binds the service if need be, then opens the tunnel; returns its file descriptor, detached. Never on the main thread. */
        fun open(context: Context): Long {
            check(VpnService.prepare(context) == null) { "Android has not let OffLok start a VPN: turn OffLok on from its window" }
            return bind(context).establishTunnel()
        }

        @Synchronized
        private fun bind(context: Context): OffLokVpnService {
            service?.let { return it }
            val bound = CountDownLatch(1)
            val conn = object : ServiceConnection {
                override fun onServiceConnected(name: ComponentName, binder: IBinder) {
                    service = (binder as AppBinder).service
                    bound.countDown()
                }

                override fun onServiceDisconnected(name: ComponentName) {
                    service = null
                }
            }
            val intent = Intent(context, OffLokVpnService::class.java).setAction(BIND_APP)
            check(context.bindService(intent, conn, Context.BIND_AUTO_CREATE)) { "OffLok's VPN service did not start" }
            connection = conn
            bound.await(10, TimeUnit.SECONDS)
            return checkNotNull(service) { "OffLok's VPN service did not start" }
        }

        /** The core closed the tunnel, or Android did: the service may end. */
        @Synchronized
        fun close(context: Context) {
            connection?.let { context.unbindService(it) }
            connection = null
            service?.stopSelf() // when "Always-on VPN" started it
            service = null
        }
    }

    private inner class AppBinder : Binder() {
        val service: OffLokVpnService get() = this@OffLokVpnService
    }

    override fun onBind(intent: Intent?): IBinder? =
        if (intent?.action == BIND_APP) AppBinder() else super.onBind(intent)

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
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
}
