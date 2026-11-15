import Foundation

enum UsageDisplayStyle: String, CaseIterable, Identifiable {
    case compact
    case gauges

    var id: String { rawValue }
    var title: String { self == .compact ? "Compact" : "Gauges" }
    var preferredWidth: CGFloat { self == .compact ? 400 : 560 }
    var menuWidth: CGFloat { self == .compact ? 400 : 440 }
    var symbol: String { self == .compact ? "rectangle.grid.1x2" : "gauge.with.dots.needle.50percent" }
}
