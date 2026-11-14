import Foundation

enum UsageDisplayStyle: String, CaseIterable, Identifiable {
    case compact
    case gauges

    var id: String { rawValue }
    var title: String { self == .compact ? "Compact" : "Gauges" }
    var preferredWidth: CGFloat { self == .compact ? 400 : 340 }
    var menuWidth: CGFloat { preferredWidth }
    var headerHeight: CGFloat { self == .compact ? 52 : 44 }
    var footerHeight: CGFloat { self == .compact ? 32 : 28 }
    var chromeHeight: CGFloat { headerHeight + footerHeight }
    var symbol: String { self == .compact ? "rectangle.grid.1x2" : "gauge.with.dots.needle.50percent" }
}
