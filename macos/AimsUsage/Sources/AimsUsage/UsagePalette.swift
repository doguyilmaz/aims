import SwiftUI

enum UsagePalette {
    static func card(_ scheme: ColorScheme) -> Color {
        scheme == .dark ? .white.opacity(0.045) : .white.opacity(0.80)
    }
    static func border(_ scheme: ColorScheme) -> Color {
        scheme == .dark ? .white.opacity(0.09) : .black.opacity(0.07)
    }
}
