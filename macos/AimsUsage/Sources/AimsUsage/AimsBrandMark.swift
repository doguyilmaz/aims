import AppKit
import SwiftUI

// The terminal chevron and cursor from assets/logo-small.svg, without the
// app-icon tile. Template alpha adapts to light/dark menu bars.
struct AimsBrandMark: Shape {
    nonisolated func path(in rect: CGRect) -> Path {
        var mark = Self.chevron()
        mark.addPath(Self.cursor(in: CGRect(x: 23, y: 23, width: 20, height: 9)))
        return Self.fitted(mark, in: rect)
    }

    nonisolated private static func chevron() -> Path {
        var chevron = Path()
        chevron.move(to: CGPoint(x: 4, y: 4))
        chevron.addLine(to: CGPoint(x: 18, y: 18))
        chevron.addLine(to: CGPoint(x: 4, y: 32))
        return chevron.strokedPath(StrokeStyle(lineWidth: 7.5, lineCap: .round, lineJoin: .round))
    }

    nonisolated private static func cursor(in rect: CGRect) -> Path {
        var cursor = Path()
        cursor.addRoundedRect(in: rect, cornerSize: CGSize(width: 1.8, height: 1.8))
        return cursor
    }

    nonisolated private static func fitted(_ path: Path, in rect: CGRect) -> Path {
        let scale = min(rect.width / 47, rect.height / 36)
        return path.applying(
            CGAffineTransform(scaleX: scale, y: scale)
                .concatenating(CGAffineTransform(translationX: rect.midX - 23.5 * scale, y: rect.midY - 18 * scale)))
    }

    static let menuImage: NSImage = {
        let size = NSSize(width: 19, height: 16)
        let image = NSImage(size: size, flipped: true) { rect in
            guard let context = NSGraphicsContext.current?.cgContext else { return false }
            NSColor.black.setFill()
            context.addPath(Self.fitted(Self.chevron(), in: rect).cgPath)
            // Keep the original cursor width: Claude on the left, Codex on
            // the right, with a sub-point gap at the native menu size.
            context.addPath(Self.fitted(Self.cursor(in: CGRect(x: 23, y: 23, width: 9, height: 9)), in: rect).cgPath)
            context.fillPath()
            NSColor.black.withAlphaComponent(0.6).setFill()
            context.addPath(Self.fitted(Self.cursor(in: CGRect(x: 34, y: 23, width: 9, height: 9)), in: rect).cgPath)
            context.fillPath()
            return true
        }
        image.isTemplate = true
        image.accessibilityDescription = "aims"
        return image
    }()
}
