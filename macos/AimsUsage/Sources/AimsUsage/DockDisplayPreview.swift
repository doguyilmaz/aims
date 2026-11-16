import SwiftUI

struct DockDisplayPreview: View {
    let display: DockDisplay
    let selected: Bool
    let side: EdgeDockSide
    let size: EdgeDockSize
    let position: Double
    let providerCount: Int

    var body: some View {
        GeometryReader { geometry in
            let layout = DockDisplayPreviewLayout(
                display: display,
                bounds: CGRect(origin: .zero, size: geometry.size).insetBy(dx: 8, dy: 7),
                side: side, size: size, position: position, providerCount: providerCount)
            ZStack(alignment: .topLeading) {
                RoundedRectangle(cornerRadius: 5)
                    .fill(
                        LinearGradient(
                            colors: [Color.accentColor.opacity(selected ? 0.23 : 0.08), .cyan.opacity(0.08)],
                            startPoint: .topLeading, endPoint: .bottomTrailing)
                    )
                    .overlay(RoundedRectangle(cornerRadius: 5).stroke(.primary.opacity(0.3), lineWidth: 2))
                    .frame(width: layout.screen.width, height: layout.screen.height)
                    .offset(x: layout.screen.minX, y: layout.screen.minY)
                Capsule().fill(.primary.opacity(0.28)).frame(width: 18, height: 3)
                    .offset(x: layout.screen.midX - 9, y: layout.screen.maxY + 4)
                RoundedRectangle(cornerRadius: 2)
                    .fill(selected ? Color.accentColor : Color.primary.opacity(0.3))
                    .overlay {
                        VStack(spacing: 2) {
                            ForEach(0..<3) { _ in Circle().fill(.white.opacity(0.75)).frame(width: 2, height: 2) }
                        }.clipped()
                    }
                    .frame(width: layout.dock.width, height: layout.dock.height)
                    .offset(x: layout.dock.minX, y: layout.dock.minY)
            }
        }
        .accessibilityHidden(true)
    }
}
