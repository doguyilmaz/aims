import Foundation

// Shared by the SwiftUI content and its native window, so page selection never changes the minimum size.
enum SettingsWindowMetrics {
    static let minimumContentSize = CGSize(width: 880, height: 660)
}
