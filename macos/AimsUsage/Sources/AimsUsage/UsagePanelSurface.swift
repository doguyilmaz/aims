import SwiftUI

/// One native material per custom Dock surface. The overview uses NSPopover’s own material.
struct UsagePanelSurface<S: Shape>: ViewModifier {
    let shape: S
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency
    @Environment(\.colorSchemeContrast) private var contrast

    @ViewBuilder func body(content: Content) -> some View {
        if reduceTransparency {
            content.clipShape(shape).background(Color(nsColor: .windowBackgroundColor), in: shape)
                .overlay {
                    shape.stroke(.primary.opacity(contrast == .increased ? 0.5 : 0.15), lineWidth: 1)
                        .allowsHitTesting(false)
                }
        } else if #available(macOS 26.0, *) {
            content.clipShape(shape).glassEffect(.regular, in: shape)
        } else {
            content.clipShape(shape).background(.regularMaterial, in: shape)
                .overlay {
                    shape.stroke(.primary.opacity(contrast == .increased ? 0.5 : 0.1), lineWidth: 1)
                        .allowsHitTesting(false)
                }
        }
    }
}
