import Foundation

enum EdgeDockMarkerStyle: String, CaseIterable, Identifiable {
    case line
    case aims
    case logo
    case pill

    init(savedValue: String?) {
        self = Self(rawValue: savedValue ?? "") ?? .line
    }

    var id: String { rawValue }

    var title: String {
        switch self {
        case .line: "Line"
        case .aims: "Aims"
        case .logo: "Logo"
        case .pill: "Pill"
        }
    }

    var hint: String {
        switch self {
        case .line: "A thin line at the screen edge."
        case .aims: "The menu-bar aims mark in a small edge tab."
        case .logo: "The same aims mark without a background."
        case .pill: "A short, rounded handle."
        }
    }

    func visualSize(for size: EdgeDockSize) -> CGSize {
        switch self {
        case .line: CGSize(width: 3, height: size == .mini ? 32 : 44)
        case .aims: CGSize(width: size == .mini ? 26 : 30, height: size == .mini ? 30 : 36)
        case .logo: CGSize(width: size == .mini ? 20 : 24, height: size == .mini ? 24 : 28)
        case .pill: CGSize(width: size == .mini ? 6 : 8, height: size == .mini ? 24 : 30)
        }
    }

    func activationWidth(for size: EdgeDockSize) -> CGFloat {
        max(EdgeDockLayout.handleWidth, visualSize(for: size).width + 4)
    }
}
