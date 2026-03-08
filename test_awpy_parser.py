from awpy.parser import DemoParser

demo_path = r"e:\fragmountdemos\college_pug_cs_office_2026-03-01_21-15.dem"

try:
    print(f"Parsing {demo_path} with awpy...")
    # Initialize the parser
    # awpy requires a backend Go parser, we will see if the API allows just reading a .dem natively
    parser = DemoParser(demofile=demo_path, parse_rate=128)
    
    # Parse the demo
    data = parser.parse()
    
    print("\n--- Parsing Successful ---")
    print(f"Total Rounds: {len(data['gameRounds'])}")
    
except Exception as e:
    print(f"\nFailed to parse demo: {e}")
