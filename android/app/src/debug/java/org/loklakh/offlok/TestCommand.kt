package org.loklakh.offlok

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log

/**
 * Debug builds only: runs one of the core's commands for the emulator test, and logs the result (tag OffLokTest):
 *   adb shell am broadcast -n org.loklakh.offlok/.TestCommand --es cmd block --es value ads=on
 */
class TestCommand : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val cmd = intent.getStringExtra("cmd") ?: return
        val value = intent.getStringExtra("value").orEmpty()
        val pending = goAsync()
        Thread {
            Log.i("OffLokTest", "$cmd $value: ${Core.call(cmd, value)}")
            pending.finish()
        }.start()
    }
}
