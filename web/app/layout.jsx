import "../styles.css";

export const metadata = {
  title: "AlgoDB Lab",
  description: "Compare data structures and trace their operations.",
};

export default function RootLayout({ children }) {
  return <html lang="en"><body>{children}</body></html>;
}
