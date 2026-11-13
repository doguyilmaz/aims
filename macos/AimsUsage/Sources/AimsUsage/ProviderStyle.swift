import SwiftUI
import UsageCore

enum ProviderStyle {
    static func tint(_ id: String) -> Color {
        switch id {
        case "claude": Color(red: 0.80, green: 0.42, blue: 0.29)
        case "codex": .teal
        case "kiro": .purple
        case "grok": .primary
        case "agy": .blue
        default: .indigo
        }
    }
    static func symbol(_ id: String) -> String {
        switch id {
        case "claude": "sun.max.fill"
        case "codex": "terminal"
        case "kiro": "sparkle"
        case "grok": "bolt.fill"
        case "agy": "sparkles"
        default: "chevron.left.forwardslash.chevron.right"
        }
    }
    static func title(_ provider: UsageProvider) -> String {
        provider.id == "claude" ? "Claude" : provider.name
    }
    static func accountName(_ row: UsageRow) -> String {
        switch row.name {
        case "personal": "Personal"
        case "work": "Work"
        case "credits": "Credits"
        case "Gemini Models": "Gemini"
        case "Claude and GPT models": "Claude / GPT"
        case "Gemini 4 Argon": "Argon 4"
        case "Gemini 4 Carbon": "Carbon 4"
        default: row.name
        }
    }

    static func planLabel(_ plan: String?) -> String? {
        guard let plan, !plan.isEmpty else { return nil }
        switch plan.lowercased() {
        case "self_serve_business_prolite", "business": return "Business"
        case "plus": return "Plus"
        case "pro": return "Pro"
        case "team": return "Team"
        case "kiro pro max": return "Pro Max"
        default: return plan.replacingOccurrences(of: "_", with: " ")
        }
    }
}
