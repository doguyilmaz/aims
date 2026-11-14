import Foundation

enum DockValueMode: String, CaseIterable, Identifiable {
    case used
    case remaining

    var id: String { rawValue }
    var title: String { self == .used ? "Used" : "Remaining" }
    var selectionDescription: String { self == .used ? "Highest used" : "Lowest remaining" }

    func percent(fromUsed used: Double) -> Double {
        self == .used ? used : max(0, 100 - used)
    }
}
