"use client"; // Tells Next.js this is an interactive browser component

import { useState } from "react";

export default function Dashboard() {
  const [prompt, setPrompt] = useState("I need a strict limit on my /api/checkout route. Keep it to exactly 30 requests per minute.");
  const [aiResponse, setAiResponse] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [testLogs, setTestLogs] = useState<{ req: number; status: number }[]>([]);

  // 1. Send the prompt to your Go server
  const handleAIConfig = async () => {
    setLoading(true);
    try {
      const res = await fetch("http://localhost:8081/admin/config/ai", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Tenant-ID": "AI_Tenant_1",
        },
        body: JSON.stringify({ prompt }),
      });
      const data = await res.json();
      setAiResponse(JSON.stringify(data, null, 2));
    } catch (error) {
      setAiResponse("Error connecting to Go server.");
    }
    setLoading(false);
  };

  // 2. Fire 35 requests rapidly to test the Rate Limiter
  const simulateTraffic = async () => {
    setTestLogs([]);
    const logs = [];
    for (let i = 1; i <= 35; i++) {
      const res = await fetch("http://localhost:8081/api/checkout", {
        method: "GET",
        headers: { "X-Tenant-ID": "AI_Tenant_1" },
      });
      logs.push({ req: i, status: res.status });
      setTestLogs([...logs]); // Update UI instantly
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 text-white p-10 font-sans">
      <h1 className="text-4xl font-bold mb-8 text-blue-400">Rate Limiter Control Plane</h1>

      <div className="grid grid-cols-2 gap-8">
        {/* Left Column: AI Configurator */}
        <div className="bg-gray-800 p-6 rounded-lg shadow-lg border border-gray-700">
          <h2 className="text-2xl font-semibold mb-4">AI Rule Generator</h2>
          <textarea
            className="w-full p-4 bg-gray-900 text-green-400 border border-gray-600 rounded mb-4 font-mono focus:outline-none focus:border-blue-500"
            rows={4}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
          />
          <button
            onClick={handleAIConfig}
            className="w-full bg-blue-600 hover:bg-blue-500 text-white font-bold py-3 px-4 rounded transition-colors"
          >
            {loading ? "Generating Configuration..." : "Deploy AI Config to Redis"}
          </button>

          {aiResponse && (
            <div className="mt-6">
              <h3 className="text-gray-400 mb-2">Active Configuration (JSON):</h3>
              <pre className="bg-black p-4 rounded text-sm text-yellow-300 overflow-x-auto">
                {aiResponse}
              </pre>
            </div>
          )}
        </div>

        {/* Right Column: Traffic Simulator */}
        <div className="bg-gray-800 p-6 rounded-lg shadow-lg border border-gray-700">
          <h2 className="text-2xl font-semibold mb-4">Traffic Load Tester</h2>
          <button
            onClick={simulateTraffic}
            className="w-full bg-red-600 hover:bg-red-500 text-white font-bold py-3 px-4 rounded mb-6 transition-colors"
          >
            Simulate 35 Requests to /api/checkout
          </button>

          <div className="h-64 overflow-y-auto bg-black p-4 rounded border border-gray-700">
            {testLogs.length === 0 ? (
              <p className="text-gray-500 text-center mt-20">No traffic detected yet.</p>
            ) : (
              testLogs.map((log) => (
                <div
                  key={log.req}
                  className={`flex justify-between p-2 mb-2 rounded ${
                    log.status === 429 ? "bg-red-900 text-red-200" : "bg-green-900 text-green-200"
                  }`}
                >
                  <span>Request #{log.req}</span>
                  <span className="font-bold">HTTP {log.status}</span>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  );
}