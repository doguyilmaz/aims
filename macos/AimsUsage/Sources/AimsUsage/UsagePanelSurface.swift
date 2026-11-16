import SwiftUI

/// One native material per floating surface. System popovers own their material.
struct UsagePanelSurface<S: Shape>: ViewModifier {
    let shape: S
    var systemPopover = false
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency
    @Environment(\.colorSchemeContrast) private var contrast

    @ViewBuilder func body(content: Content) -> some View {
        if systemPopover {
            content
        } else if reduceTransparency {
            content.background(Color(nsColor: .windowBackgroundColor), in: shape)
                .overlay {
                    shape.stroke(.primary.opacity(contrast == .increased ? 0.5 : 0.15), lineWidth: 1)
                        .allowsHitTesting(false)
                }
        } else if #available(macOS 26.0, *) {
            content.glassEffect(.regular, in: shape)
        } else {
            content.background(.regularMaterial, in: shape)
                .overlay {
                    shape.stroke(.primary.opacity(contrast == .increased ? 0.5 : 0.1), lineWidth: 1)
                        .allowsHitTesting(false)
                }
        }
    }
}
