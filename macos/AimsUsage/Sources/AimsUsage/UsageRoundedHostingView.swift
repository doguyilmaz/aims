import AppKit
import SwiftUI

/// Keep native hosting/effect layers inside the same outline as the SwiftUI surface.
final class UsageRoundedHostingView: NSView {
    private let host: NSHostingView<AnyView>
    var rootView: AnyView {
        get { host.rootView }
        set { host.rootView = newValue }
    }
    var cornerRadius: CGFloat {
        didSet { updateOutline() }
    }
    private var shadowBounds: CGRect?
    private var shadowCornerRadius: CGFloat?

    init(rootView: AnyView, cornerRadius: CGFloat) {
        host = NSHostingView(rootView: rootView)
        self.cornerRadius = cornerRadius
        super.init(frame: .zero)
        wantsLayer = true
        clipsToBounds = true
        host.translatesAutoresizingMaskIntoConstraints = false
        addSubview(host)
        NSLayoutConstraint.activate([
            host.leadingAnchor.constraint(equalTo: leadingAnchor),
            host.trailingAnchor.constraint(equalTo: trailingAnchor),
            host.topAnchor.constraint(equalTo: topAnchor),
            host.bottomAnchor.constraint(equalTo: bottomAnchor),
        ])
        updateOutline()
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
