from csgo.parser import DemoParser

demo_path = r"e:\fragmountdemos\college_pug_cs_office_2026-03-01_21-15.dem"

try:
    print(f"Parsing {demo_path}...")
    # Initialize the parser
    parser = DemoParser(demofile=demo_path, match_id="test_demo", parse_rate=128)
    
    # Parse the demo
    data = parser.parse()
    
    print("\n--- Parsing Successful ---")
    print(f"Map: {data['mapName']}")
    print(f"Tickrate: {data['tickRate']}")
    print(f"Server Name: {data['serverName']}")
    print(f"Client Name: {data['clientName']}")
    print(f"Total Ticks: {data['playbackTicks']}")
    
except Exception as e:
    print(f"\nFailed to parse demo: {e}")
