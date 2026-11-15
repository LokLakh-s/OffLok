package org.loklakh.offlok

import android.content.Context

/**
 * Google Play's prominent disclosure: OffLok's window says what its VPN does, and the user accepts it, before Android
 * first asks for the VPN. Until then the window's "on" answers with "disclosure", and the tile opens the window.
 */
object Disclosure {
    private const val PREFS = "disclosure"

    fun accepted(context: Context): Boolean =
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getBoolean("accepted", false)

    fun accept(context: Context) =
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().putBoolean("accepted", true).apply()
}
