// Draws an app's icon as macOS shows it (NSWorkspace, through IconServices), 512 px, into a PNG: the macOS
// workflow keeps it next to the package, to see whether macOS 26 takes OffLok's tile as it is or shuts it in a grey
// square, as it does with icons that do not follow its shape.
//   swift test/macos/render-icon.swift <app, its full path> <out.png>
import AppKit

let args = CommandLine.arguments
guard args.count == 3 else {
    FileHandle.standardError.write("usage: render-icon.swift <app> <out.png>\n".data(using: .utf8)!)
    exit(2)
}
let icon = NSWorkspace.shared.icon(forFile: args[1])
let size = 512
guard let rep = NSBitmapImageRep(
    bitmapDataPlanes: nil, pixelsWide: size, pixelsHigh: size, bitsPerSample: 8, samplesPerPixel: 4,
    hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)
else { exit(1) }
NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
icon.draw(in: NSRect(x: 0, y: 0, width: size, height: size))
NSGraphicsContext.restoreGraphicsState()
guard let png = rep.representation(using: .png, properties: [:]) else { exit(1) }
try png.write(to: URL(fileURLWithPath: args[2]))
