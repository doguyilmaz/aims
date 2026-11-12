import SwiftUI

struct UsageBackdrop: View {
    @Environment(\.colorScheme) private var scheme
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency

    var body: some View {
        ZStack {
            if !reduceTransparency { UsageMaterial() }
            UsagePalette.ink(scheme).opacity(reduceTransparency ? 1 : 0.88)
            LinearGradient(
                colors: [.cyan.opacity(scheme == .dark ? 0.055 : 0.035), .clear],
                startPoint: .topLeading, endPoint: .bottomTrailing)
        }.allowsHitTesting(false)
    }
}
