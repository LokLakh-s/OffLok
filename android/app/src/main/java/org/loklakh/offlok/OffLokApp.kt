package org.loklakh.offlok

import android.app.Application

/** Starts OffLok's core with the process, whichever part of the app Android starts first. */
class OffLokApp : Application() {
    override fun onCreate() {
        super.onCreate()
        Core.start(this)
    }
}
