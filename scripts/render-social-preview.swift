import AppKit

// Render the repository/site social card from the existing app mark.
let output = CommandLine.arguments.dropFirst().first ?? "assets/social-preview.png"
let width = 1200
let height = 630
let bitmap = NSBitmapImageRep(
    bitmapDataPlanes: nil, pixelsWide: width, pixelsHigh: height,
    bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
    colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
let graphics = NSGraphicsContext(bitmapImageRep: bitmap)!
NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = graphics
let bounds = NSRect(x: 0, y: 0, width: width, height: height)
NSColor(srgbRed: 18 / 255, green: 17 / 255, blue: 16 / 255, alpha: 1).setFill()
bounds.fill()
let coral = NSColor(srgbRed: 217 / 255, green: 119 / 255, blue: 87 / 255, alpha: 1)
let ivory = NSColor(srgbRed: 240 / 255, green: 238 / 255, blue: 230 / 255, alpha: 1)
let secondary = NSColor(srgbRed: 0.77, green: 0.75, blue: 0.72, alpha: 1)
let gradient = NSGradient(starting: coral.withAlphaComponent(0.14), ending: coral.withAlphaComponent(0))!
gradient.draw(in: NSBezierPath(ovalIn: NSRect(x: -350, y: -150, width: 1100, height: 1100)),
    relativeCenterPosition: .zero)
let logo = NSImage(contentsOfFile: "assets/logo-512.png")!
logo.draw(in: NSRect(x: 80, y: 172, width: 288, height: 288))

func text(_ value: String, top: CGFloat, size: CGFloat, weight: NSFont.Weight, color: NSColor) {
    let attributes: [NSAttributedString.Key: Any] = [
        .font: NSFont.systemFont(ofSize: size, weight: weight), .foregroundColor: color,
    ]
    (value as NSString).draw(in: NSRect(x: 430, y: 630 - top - size * 1.3, width: 745, height: size * 1.3),
        withAttributes: attributes)
}
text("aims", top: 88, size: 34, weight: .bold, color: coral)
text("AI accounts.", top: 171, size: 66, weight: .bold, color: ivory)
text("Usage at a glance.", top: 249, size: 66, weight: .bold, color: ivory)
text("Claude Code and Codex, personal and work.", top: 360, size: 27, weight: .regular, color: secondary)
text("Sessions, quotas, resets and budgets.", top: 406, size: 25, weight: .regular, color: secondary)
text("CLI  ·  Menu bar  ·  Edge Dock", top: 486, size: 23, weight: .medium, color: coral)
text("aims.doguyilmaz.com", top: 534, size: 23, weight: .regular, color: secondary)
NSGraphicsContext.restoreGraphicsState()
try bitmap.representation(using: .png, properties: [:])!.write(to: URL(filePath: output))
