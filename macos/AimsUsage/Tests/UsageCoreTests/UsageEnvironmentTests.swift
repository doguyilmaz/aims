import Foundation
import Testing

@testable import UsageCore

@Test func customDataHomeAndProviderPathsSurviveFinderEnvironment() throws {
    let environment = try UsageEnvironment.make(
        base: ["PATH": "/usr/bin:/bin"], homeDirectory: "/fixture/home", aimsHome: "~/work-aims",
        additionalPaths: "~/tools/bin\n/opt/custom/bin")
    #expect(environment["AIMS_HOME"] == "/fixture/home/work-aims")
    let paths = try #require(environment["PATH"]).split(separator: ":").map(String.init)
    #expect(Array(paths.prefix(2)) == ["/fixture/home/tools/bin", "/opt/custom/bin"])
    for directory in ["go/bin", ".cargo/bin", ".npm-global/bin", "bin"] {
        #expect(paths.contains("/fixture/home/" + directory))
    }
    #expect(!paths.contains(""))
}

@Test func defaultFolderExplicitlyClearsInheritedOverride() throws {
    let environment = try UsageEnvironment.make(
        base: ["AIMS_HOME": "/fixture/old", "PATH": ":.:/usr/bin::"], homeDirectory: "/fixture/home", aimsHome: "",
        additionalPaths: "")
    #expect(environment["AIMS_HOME"] == nil)
    #expect(!(environment["PATH"] ?? "").contains("::"))
    #expect(!(environment["PATH"] ?? "").split(separator: ":").contains("."))
    #expect(throws: UsageError.self) {
        try UsageEnvironment.make(
            base: [:], homeDirectory: "/fixture/home", aimsHome: "relative/path", additionalPaths: "")
    }
}

@Test func cappedResetDetailsStayVisiblyIncomplete() throws {
    let decoder = JSONDecoder()
    let capped = try decoder.decode(ResetBalance.self, from: Data(#"{"available":2,"credits":[]}"#.utf8))
    let empty = try decoder.decode(ResetBalance.self, from: Data(#"{"available":0,"credits":null}"#.utf8))
    #expect(capped.detailsAreIncomplete)
    #expect(capped.detailCount == 0)
    #expect(!empty.detailsAreIncomplete)
}
