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
                            colors: selected
                                ? [Color.accentColor.opacity(0.8), .cyan.opacity(0.4)]
                                : [.primary.opacity(0.06), .primary.opacity(0.02)],
                            startPoint: .topLeading, endPoint: .bottomTrailing)
                    )
                    .overlay {
                        RoundedRectangle(cornerRadius: 5)
                            .stroke(selected ? Color.accentColor : .primary.opacity(0.3), lineWidth: 2)
                    }
                    .frame(width: layout.screen.width, height: layout.screen.height)
                    .offset(x: layout.screen.minX, y: layout.screen.minY)
                Capsule().fill(selected ? Color.accentColor.opacity(0.7) : .primary.opacity(0.28))
                    .frame(width: 18, height: 3)
                    .offset(x: layout.screen.midX - 9, y: layout.screen.maxY + 4)
                if selected && !layout.screen.isEmpty {
                    let dockWidth = max(3, layout.dock.width)
                    RoundedRectangle(cornerRadius: 1.5)
                        .fill(.white)
                        .overlay {
                            VStack(spacing: 1) {
                                ForEach(0..<3) { _ in Circle().fill(Color.accentColor).frame(width: 1, height: 1) }
                            }.clipped()
                        }
                        .frame(width: dockWidth, height: layout.dock.height)
                        .offset(x: side == .left ? layout.dock.minX : layout.dock.maxX - dockWidth, y: layout.dock.minY)
                    Image(systemName: "checkmark.circle.fill")
                        .symbolRenderingMode(.palette).foregroundStyle(.white, Color.accentColor)
                        .font(.system(size: 14, weight: .semibold)).frame(width: 14, height: 14)
                        .offset(
                            x: (side == .right ? layout.screen.minX : layout.screen.maxX) - 7,
                            y: layout.screen.minY - 7)
                }
            }
        }
        .accessibilityHidden(true)
    }
}
