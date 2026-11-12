import Foundation

public enum UsageEnvironment {
    public static func homePath(_ path: String, homeDirectory: String) throws -> String {
        let trimmed = path.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty { return "" }
        let expanded =
            trimmed == "~"
            ? homeDirectory : trimmed.hasPrefix("~/") ? homeDirectory + "/" + trimmed.dropFirst(2) : trimmed
        guard expanded.hasPrefix("/") else { throw UsageError.invalidDataFolder }
        return (expanded as NSString).standardizingPath
    }

    public static func make(base: [String: String], homeDirectory: String, aimsHome: String, additionalPaths: String)
        throws -> [String: String]
    {
        var environment = base
        let selectedHome = try homePath(aimsHome, homeDirectory: homeDirectory)
        if selectedHome.isEmpty {
            environment.removeValue(forKey: "AIMS_HOME")
        } else {
            environment["AIMS_HOME"] = selectedHome
        }
        let custom = try additionalPaths.components(separatedBy: .newlines).map {
            try homePath($0, homeDirectory: homeDirectory)
        }.filter { !$0.isEmpty }
        let common = [
            ".local/bin", "go/bin", ".cargo/bin", ".npm-global/bin", "bin", ".grok/bin", ".opencode/bin", ".kiro/bin",
            ".bun/bin", ".volta/bin", "Library/pnpm",
        ].map { homeDirectory + "/" + $0 }
        let inherited = (base["PATH"] ?? "").split(separator: ":").map(String.init).filter { $0.hasPrefix("/") }
        var seen = Set<String>()
        environment["PATH"] =
            (custom + common + [
                "/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin",
            ] + inherited).filter { seen.insert($0).inserted }.joined(separator: ":")
        environment["AIMS_NO_UPDATE_CHECK"] = "1"
        environment["NO_COLOR"] = "1"
        return environment
    }
}
