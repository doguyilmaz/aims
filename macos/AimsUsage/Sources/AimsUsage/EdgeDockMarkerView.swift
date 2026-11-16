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
                mark
                    .frame(width: dimensions.width, height: dimensions.height)
                    .usagePanelSurface(
                        in: UnevenRoundedRectangle(
                            topLeadingRadius: side == .right ? 8 : 2,
                            bottomLeadingRadius: side == .right ? 8 : 2,
                            bottomTrailingRadius: side == .left ? 8 : 2,
                            topTrailingRadius: side == .left ? 8 : 2))
            case .logo:
                mark.shadow(color: Color(nsColor: .windowBackgroundColor).opacity(0.8), radius: 1)
            case .pill:
                Capsule().fill(Color.primary.opacity(0.65))
            }
        }
        .frame(width: dimensions.width, height: dimensions.height)
        .accessibilityHidden(true)
    }

    private var mark: some View {
        Image(nsImage: AimsBrandMark.menuImage).resizable().scaledToFit()
            .foregroundStyle(.primary)
            .frame(width: size == .mini ? 16 : 19, height: size == .mini ? 14 : 16)
    }
}
