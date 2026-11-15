import SwiftUI

enum UsagePalette {
    static func ink(_ scheme: ColorScheme) -> Color {
        scheme == .dark ? Color(red: 0.045, green: 0.065, blue: 0.10) : Color(red: 0.95, green: 0.97, blue: 1)
    }
    static func card(_ scheme: ColorScheme) -> Color {
        scheme == .dark ? .white.opacity(0.045) : .white.opacity(0.80)
    }
    static func border(_ scheme: ColorScheme) -> Color {
        scheme == .dark ? .white.opacity(0.09) : .black.opacity(0.07)
    }
}
