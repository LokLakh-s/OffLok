package org.loklakh.offlok

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log

/**
 * Debug builds only: runs one of the core's commands for the emulator test, and logs the result (tag OffLokTest),
 * after the id the test gave:
 *   adb shell am broadcast -n org.loklakh.offlok/.TestCommand --es id 7 --es cmd block --es value ads=on
 */
class TestCommand : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val cmd = intent.getStringExtra("cmd") ?: return
        val value = intent.getStringExtra("value").orEmpty()
        val id = intent.getStringExtra("id").orEmpty()
        val pending = goAsync()
        Thread {
            Log.i("OffLokTest", "#$id $cmd $value: ${Core.call(cmd, value)}")
            pending.finish()
        }.start()
    }
}
