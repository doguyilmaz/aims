import SwiftUI

struct ContentHeightKey: PreferenceKey {
    nonisolated static var defaultValue: CGFloat { 0 }
    nonisolated static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) { value = max(value, nextValue()) }
}
