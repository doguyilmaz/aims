import Foundation

enum EdgeDockSize: String, CaseIterable, Identifiable {
    case standard
    case mini

    var id: String { rawValue }
    var title: String { self == .standard ? "Standard" : "Mini" }
    var width: CGFloat { self == .standard ? 64 : 44 }
    var rowStride: CGFloat { self == .standard ? 64 : 48 }
    var contentInset: CGFloat { self == .standard ? 32 : 24 }
    var ringSize: CGFloat { self == .standard ? 38 : 28 }
    var iconSize: CGFloat { self == .standard ? 19 : 14 }
    var labelSize: CGFloat { self == .standard ? 12 : 11 }
    var chromeHeight: CGFloat { contentInset * 2 + 24 }
}
