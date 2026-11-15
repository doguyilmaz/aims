import AppKit
import SwiftUI

// The terminal chevron and cursor from assets/logo-small.svg, without the
// app-icon tile. One template color adapts to light/dark menu bars.
struct AimsBrandMark: Shape {
    nonisolated func path(in rect: CGRect) -> Path {
        var chevron = Path()
        chevron.move(to: CGPoint(x: 4, y: 4))
        chevron.addLine(to: CGPoint(x: 18, y: 18))
        chevron.addLine(to: CGPoint(x: 4, y: 32))
        var mark = chevron.strokedPath(StrokeStyle(lineWidth: 7.5, lineCap: .round, lineJoin: .round))
        mark.addRoundedRect(in: CGRect(x: 23, y: 23, width: 20, height: 9), cornerSize: CGSize(width: 1.8, height: 1.8))
        let scale = min(rect.width / 47, rect.height / 36)
        return mark.applying(
            CGAffineTransform(scaleX: scale, y: scale)
                .concatenating(CGAffineTransform(translationX: rect.midX - 23.5 * scale, y: rect.midY - 18 * scale)))
    }

    static let menuImage: NSImage = {
        let size = NSSize(width: 19, height: 16)
        let image = NSImage(size: size, flipped: true) { rect in
            NSColor.black.setFill()
            let context = NSGraphicsContext.current?.cgContext
            context?.addPath(AimsBrandMark().path(in: rect).cgPath)
            context?.fillPath()
            return true
        }
        image.isTemplate = true
        image.accessibilityDescription = "aims"
        return image
    }()
}
