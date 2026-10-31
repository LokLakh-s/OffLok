package org.loklakh.offlok

import android.content.Context
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import org.loklakh.offlok.mobile.Mobile

/**
 * Tells the core the network the phone uses: its DNS servers and search domains (the fallback, captive portals,
 * local names), its Private DNS hostname, and when it changes. OffLok is left out of its own VPN: its default
 * network is the phone's, never its tunnel.
 */
object NetworkWatch {
    fun start(context: Context) {
        val cm = context.getSystemService(ConnectivityManager::class.java)
        cm.registerDefaultNetworkCallback(object : ConnectivityManager.NetworkCallback() {
            override fun onLinkPropertiesChanged(network: Network, lp: LinkProperties) = report(network, lp)

            override fun onLost(network: Network) = Mobile.setNetwork("", "", "", "")
        })
    }

    private fun report(network: Network, lp: LinkProperties) {
        val servers = lp.dnsServers.mapNotNull { it.hostAddress }.joinToString("\n")
        val domains = lp.domains.orEmpty().split(',', ' ').filter { it.isNotBlank() }.joinToString("\n")
        // A new network, or the same one with new addresses: the core probes the root servers again.
        val id = "$network ${lp.interfaceName} ${lp.linkAddresses.joinToString(",")}"
        // A hostname set in Android's Private DNS setting sends every query to it, around any VPN.
        val privateDns = if (lp.isPrivateDnsActive) lp.privateDnsServerName.orEmpty() else ""
        Mobile.setNetwork(id, servers, domains, privateDns)
    }
}
