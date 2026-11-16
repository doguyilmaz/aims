import SwiftUI

struct EdgeDockMarkerView: View {
    let style: EdgeDockMarkerStyle
    let side: EdgeDockSide
    let size: EdgeDockSize

    var body: some View {
        let dimensions = style.visualSize(for: size)
        Group {
            switch style {
            case .line:
                Capsule().fill(.gray.opacity(0.75))
            case .aims:
                AimsBrandMark().fill(Color.primary.opacity(0.9))
                    .frame(width: size == .mini ? 16 : 18, height: size == .mini ? 13 : 15)
                    .frame(width: dimensions.width, height: dimensions.height)
                    .usagePanelSurface(
                        in: UnevenRoundedRectangle(
                            topLeadingRadius: side == .right ? 8 : 2,
                            bottomLeadingRadius: side == .right ? 8 : 2,
                            bottomTrailingRadius: side == .left ? 8 : 2,
                            topTrailingRadius: side == .left ? 8 : 2))
            case .pill:
                Capsule().fill(Color.primary.opacity(0.65))
            }
        }
        .frame(width: dimensions.width, height: dimensions.height)
        .accessibilityHidden(true)
    }
}
