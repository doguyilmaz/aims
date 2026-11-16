import AppKit
import SwiftUI

/// Keep native hosting/effect layers inside the same outline as the SwiftUI surface.
final class UsageRoundedHostingView<Content: View>: NSHostingView<Content> {
    var cornerRadius: CGFloat = 0 {
        didSet { updateOutline() }
    }
    private var shadowBounds: CGRect?
    private var shadowCornerRadius: CGFloat?

    required init(rootView: Content) {
        super.init(rootView: rootView)
        wantsLayer = true
        clipsToBounds = true
        updateOutline()
    }

    convenience init(rootView: Content, cornerRadius: CGFloat) {
        self.init(rootView: rootView)
        self.cornerRadius = cornerRadius
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { nil }

    override func layout() {
        super.layout()
        updateOutline()
    }

    private func updateOutline() {
        layer?.cornerRadius = cornerRadius
        layer?.cornerCurve = .continuous
        layer?.masksToBounds = true
        if shadowBounds != bounds || shadowCornerRadius != cornerRadius {
            shadowBounds = bounds
            shadowCornerRadius = cornerRadius
            window?.invalidateShadow()
        }
    }
}
