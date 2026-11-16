import Foundation

/// A value snapshot, never a retained NSScreen. Coordinates are global AppKit points.
struct DockDisplay: Identifiable, Equatable {
    let id: String
    let name: String
    let frame: CGRect
    let visibleFrame: CGRect
}
