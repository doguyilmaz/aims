enum UsageDisplayStyle: String, CaseIterable, Identifiable {
    case compact
    case gauges

    var id: String { rawValue }
    var title: String { self == .compact ? "Compact" : "Gauges" }
    var symbol: String { self == .compact ? "rectangle.grid.1x2" : "gauge.with.dots.needle.50percent" }
}
